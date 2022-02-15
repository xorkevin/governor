package org

import (
	"context"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/rank"
)

type (
	// Orgs is an organization management service
	Orgs interface {
		GetByID(orgid string) (*ResOrg, error)
		GetByName(name string) (*ResOrg, error)
	}

	// Service is an Orgs and governor.Service
	Service interface {
		governor.Service
		Orgs
	}

	service struct {
		orgs        model.Repo
		roles       role.Roles
		users       user.Users
		events      events.Events
		ratelimiter ratelimit.Ratelimiter
		gate        gate.Gate
		logger      governor.Logger
		scopens     string
		streamns    string
		opts        Opts
		streamsize  int64
		eventsize   int32
		roleopts    role.Opts
		useropts    user.Opts
	}

	router struct {
		s  *service
		rt governor.Middleware
	}

	// DeleteOrgProps are properties of a deleted org
	DeleteOrgProps struct {
		OrgID string `json:"orgid"`
	}

	ctxKeyOrgs struct{}

	Opts struct {
		StreamName    string
		DeleteChannel string
	}

	ctxKeyOpts struct{}
)

// GetCtxOrgs returns an Orgs service from the context
func GetCtxOrgs(inj governor.Injector) Orgs {
	v := inj.Get(ctxKeyOrgs{})
	if v == nil {
		return nil
	}
	return v.(Orgs)
}

// setCtxOrgs sets an Orgs service in the context
func setCtxOrgs(inj governor.Injector, o Orgs) {
	inj.Set(ctxKeyOrgs{}, o)
}

// GetCtxOpts returns org Opts from the context
func GetCtxOpts(inj governor.Injector) Opts {
	v := inj.Get(ctxKeyOpts{})
	if v == nil {
		return Opts{}
	}
	return v.(Opts)
}

// SetCtxOpts sets org Opts in the context
func SetCtxOpts(inj governor.Injector, o Opts) {
	inj.Set(ctxKeyOpts{}, o)
}

// NewCtx creates a new Orgs service from a context
func NewCtx(inj governor.Injector) Service {
	orgs := model.GetCtxRepo(inj)
	roles := role.GetCtxRoles(inj)
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	roleopts := role.GetCtxOpts(inj)
	useropts := user.GetCtxOpts(inj)
	return New(orgs, roles, users, ev, ratelimiter, g, roleopts, useropts)
}

// New returns a new Orgs service
func New(orgs model.Repo, roles role.Roles, users user.Users, ev events.Events, ratelimiter ratelimit.Ratelimiter, g gate.Gate, roleopts role.Opts, useropts user.Opts) Service {
	return &service{
		orgs:        orgs,
		roles:       roles,
		users:       users,
		events:      ev,
		ratelimiter: ratelimiter,
		gate:        g,
		roleopts:    roleopts,
		useropts:    useropts,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxOrgs(inj, s)
	s.scopens = "gov." + name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = Opts{
		StreamName:    streamname,
		DeleteChannel: streamname + ".delete",
	}
	SetCtxOpts(inj, s.opts)

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
}

func (s *service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.Base(),
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)

	l.Info("Loaded config", map[string]string{
		"stream size (bytes)": r.GetStr("streamsize"),
		"event size (bytes)":  r.GetStr("eventsize"),
	})

	sr := s.router()
	sr.mountRoute(m)
	l.Info("Mounted http routes", nil)

	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.events.InitStream(s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to init org stream")
	}
	l.Info("Created org stream", nil)

	if err := s.orgs.Setup(); err != nil {
		return err
	}
	l.Info("Created userorgs table", nil)

	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DeleteChannel, s.streamns+"_WORKER_ROLE_DELETE", s.OrgRoleDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to org delete queue")
	}
	l.Info("Subscribed to org delete queue", nil)

	if _, err := s.events.StreamSubscribe(s.roleopts.StreamName, s.roleopts.CreateChannel, s.streamns+"_WORKER_USER_ROLE_CREATE", s.UserRoleCreate, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user role create queue")
	}
	l.Info("Subscribed to user role create queue", nil)

	if _, err := s.events.StreamSubscribe(s.roleopts.StreamName, s.roleopts.DeleteChannel, s.streamns+"_WORKER_USER_ROLE_DELETE", s.UserRoleDelete, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user role delete queue")
	}
	l.Info("Subscribed to user role delete queue", nil)

	if _, err := s.events.StreamSubscribe(s.useropts.StreamName, s.useropts.UpdateChannel, s.streamns+"_WORKER_USER_UPDATE", s.UserUpdateHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user update queue")
	}
	l.Info("Subscribed to user update queue", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

const (
	roleDeleteBatchSize = 256
)

// OrgRoleDeleteHook deletes the roles of a deleted org
func (s *service) OrgRoleDeleteHook(pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := DecodeDeleteOrgProps(msgdata)
	if err != nil {
		return err
	}
	orgrole := rank.ToOrgName(props.OrgID)
	usrOrgrole := rank.ToUsrName(orgrole)
	modOrgrole := rank.ToModName(orgrole)
	if err := s.users.DeleteRoleInvitations(modOrgrole); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete role mod invitations")
	}
	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		userids, err := s.roles.GetByRole(modOrgrole, roleDeleteBatchSize, 0)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get role mods")
		}
		if len(userids) == 0 {
			break
		}
		if err := s.roles.DeleteByRole(modOrgrole, userids); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete role mods")
		}
		if len(userids) < roleDeleteBatchSize {
			break
		}
	}
	if err := s.users.DeleteRoleInvitations(usrOrgrole); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete role user invitations")
	}
	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		userids, err := s.roles.GetByRole(usrOrgrole, roleDeleteBatchSize, 0)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get role users")
		}
		if len(userids) == 0 {
			break
		}
		if err := s.roles.DeleteByRole(usrOrgrole, userids); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete role users")
		}
		if len(userids) < roleDeleteBatchSize {
			break
		}
	}
	return nil
}

func (s *service) UserRoleCreate(pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := role.DecodeRolesProps(msgdata)
	if err != nil {
		return err
	}

	u, err := s.users.GetByID(props.Userid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil
		}
		return governor.ErrWithMsg(err, "Failed to get user")
	}

	allorgids := map[string]struct{}{}
	memberorgids := map[string]struct{}{}
	modorgids := map[string]struct{}{}
	for _, i := range props.Roles {
		if strings.HasPrefix(i, rank.PrefixUsrOrg) {
			k := strings.TrimPrefix(i, rank.PrefixUsrOrg)
			memberorgids[k] = struct{}{}
			allorgids[k] = struct{}{}
			continue
		}
		if strings.HasPrefix(i, rank.PrefixModOrg) {
			k := strings.TrimPrefix(i, rank.PrefixModOrg)
			modorgids[k] = struct{}{}
			allorgids[k] = struct{}{}
			continue
		}
	}
	orgids := make([]string, 0, len(allorgids))
	for k := range allorgids {
		orgids = append(orgids, k)
	}

	m, err := s.orgs.GetOrgs(orgids)
	if err != nil {
		return governor.ErrWithMsg(err, "Failed to get orgs")
	}

	members := make([]*model.MemberModel, 0, len(memberorgids))
	mods := make([]*model.MemberModel, 0, len(modorgids))
	for _, i := range m {
		if _, ok := memberorgids[i.OrgID]; ok {
			members = append(members, &model.MemberModel{
				OrgID:    i.OrgID,
				Userid:   props.Userid,
				Name:     i.Name,
				Username: u.Username,
			})
		}
		if _, ok := modorgids[i.OrgID]; ok {
			mods = append(mods, &model.MemberModel{
				OrgID:    i.OrgID,
				Userid:   props.Userid,
				Name:     i.Name,
				Username: u.Username,
			})
		}
	}

	if err := s.orgs.AddMembers(members); err != nil {
		return governor.ErrWithMsg(err, "Failed to add org members")
	}
	if err := s.orgs.AddMods(mods); err != nil {
		return governor.ErrWithMsg(err, "Failed to add org mods")
	}

	return nil
}

func (s *service) UserRoleDelete(pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := role.DecodeRolesProps(msgdata)
	if err != nil {
		return err
	}

	memberorgids := make([]string, 0, len(props.Roles))
	modorgids := make([]string, 0, len(props.Roles))
	for _, i := range props.Roles {
		if strings.HasPrefix(i, rank.PrefixUsrOrg) {
			memberorgids = append(memberorgids, strings.TrimPrefix(i, rank.PrefixUsrOrg))
			continue
		}
		if strings.HasPrefix(i, rank.PrefixModOrg) {
			modorgids = append(modorgids, strings.TrimPrefix(i, rank.PrefixModOrg))
			continue
		}
	}
	if err := s.orgs.RmMembers(props.Userid, memberorgids); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove org members")
	}
	if err := s.orgs.RmMods(props.Userid, modorgids); err != nil {
		return governor.ErrWithMsg(err, "Failed to remove org mods")
	}

	return nil
}

// UserUpdateHook updates a user name
func (s *service) UserUpdateHook(pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := user.DecodeUpdateUserProps(msgdata)
	if err != nil {
		return err
	}
	if err := s.orgs.UpdateUsername(props.Userid, props.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to update member username")
	}
	return nil
}
