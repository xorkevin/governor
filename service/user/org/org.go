package org

import (
	"context"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// WorkerFuncDelete is a type alias for a delete org event consumer
	WorkerFuncDelete = func(ctx context.Context, pinger events.Pinger, props DeleteOrgProps) error

	// Orgs is an organization management service
	Orgs interface {
		GetByID(ctx context.Context, orgid string) (*ResOrg, error)
		GetByName(ctx context.Context, name string) (*ResOrg, error)
		StreamSubscribeDelete(group string, worker WorkerFuncDelete, streamopts events.StreamConsumerOpts) (events.Subscription, error)
	}

	Service struct {
		orgs        model.Repo
		roles       role.Roles
		users       user.Users
		events      events.Events
		ratelimiter ratelimit.Ratelimiter
		gate        gate.Gate
		log         *klog.LevelLogger
		scopens     string
		streamns    string
		opts        svcOpts
		streamsize  int64
		eventsize   int32
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}

	// DeleteOrgProps are properties of a deleted org
	DeleteOrgProps struct {
		OrgID string `json:"orgid"`
	}

	ctxKeyOrgs struct{}

	svcOpts struct {
		StreamName    string
		DeleteChannel string
	}
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

// NewCtx creates a new Orgs service from a context
func NewCtx(inj governor.Injector) *Service {
	orgs := model.GetCtxRepo(inj)
	roles := role.GetCtxRoles(inj)
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	return New(orgs, roles, users, ev, ratelimiter, g)
}

// New returns a new Orgs service
func New(orgs model.Repo, roles role.Roles, users user.Users, ev events.Events, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
	return &Service{
		orgs:        orgs,
		roles:       roles,
		users:       users,
		events:      ev,
		ratelimiter: ratelimiter,
		gate:        g,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxOrgs(inj, s)
	s.scopens = "gov." + name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = svcOpts{
		StreamName:    streamname,
		DeleteChannel: streamname + ".delete",
	}

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"org.stream.size": r.GetStr("streamsize"),
		"org.event.size":  r.GetStr("eventsize"),
	})

	sr := s.router()
	sr.mountRoute(m)
	s.log.Info(ctx, "Mounted http routes", nil)

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	if _, err := s.StreamSubscribeDelete(s.streamns+"_WORKER_ROLE_DELETE", s.orgRoleDeleteHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to org delete queue")
	}
	s.log.Info(ctx, "Subscribed to org delete queue", nil)

	if _, err := s.roles.StreamSubscribeCreate(s.streamns+"_WORKER_USER_ROLE_CREATE", s.userRoleCreate, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user role create queue")
	}
	s.log.Info(ctx, "Subscribed to user role create queue", nil)

	if _, err := s.roles.StreamSubscribeDelete(s.streamns+"_WORKER_USER_ROLE_DELETE", s.userRoleDelete, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user role delete queue")
	}
	s.log.Info(ctx, "Subscribed to user role delete queue", nil)

	if _, err := s.users.StreamSubscribeUpdate(s.streamns+"_WORKER_USER_UPDATE", s.userUpdateHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user update queue")
	}
	s.log.Info(ctx, "Subscribed to user update queue", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init org stream")
	}
	s.log.Info(ctx, "Created org stream", nil)

	if err := s.orgs.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userorgs table", nil)

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func decodeDeleteOrgProps(msgdata []byte) (*DeleteOrgProps, error) {
	m := &DeleteOrgProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode delete org props")
	}
	return m, nil
}

func (s *Service) StreamSubscribeDelete(group string, worker WorkerFuncDelete, streamopts events.StreamConsumerOpts) (events.Subscription, error) {
	sub, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DeleteChannel, group, func(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
		props, err := decodeDeleteOrgProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, pinger, *props)
	}, streamopts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to user delete channel")
	}
	return sub, nil
}

const (
	roleDeleteBatchSize = 256
)

func (s *Service) orgRoleDeleteHook(ctx context.Context, pinger events.Pinger, props DeleteOrgProps) error {
	orgrole := rank.ToOrgName(props.OrgID)
	usrOrgrole := rank.ToUsrName(orgrole)
	modOrgrole := rank.ToModName(orgrole)
	if err := s.users.DeleteRoleInvitations(ctx, modOrgrole); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role mod invitations")
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		userids, err := s.roles.GetByRole(ctx, modOrgrole, roleDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get role mods")
		}
		if len(userids) == 0 {
			break
		}
		if err := s.roles.DeleteByRole(ctx, modOrgrole, userids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete role mods")
		}
		if len(userids) < roleDeleteBatchSize {
			break
		}
	}
	if err := s.users.DeleteRoleInvitations(ctx, usrOrgrole); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role user invitations")
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		userids, err := s.roles.GetByRole(ctx, usrOrgrole, roleDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get role users")
		}
		if len(userids) == 0 {
			break
		}
		if err := s.roles.DeleteByRole(ctx, usrOrgrole, userids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete role users")
		}
		if len(userids) < roleDeleteBatchSize {
			break
		}
	}
	return nil
}

func (s *Service) userRoleCreate(ctx context.Context, pinger events.Pinger, props role.RolesProps) error {
	u, err := s.users.GetByID(ctx, props.Userid)
	if err != nil {
		if errors.Is(err, user.ErrorNotFound{}) {
			return nil
		}
		return kerrors.WithMsg(err, "Failed to get user")
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

	m, err := s.orgs.GetOrgs(ctx, orgids)
	if err != nil {
		return kerrors.WithMsg(err, "Failed to get orgs")
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

	if err := s.orgs.AddMembers(ctx, members); err != nil {
		return kerrors.WithMsg(err, "Failed to add org members")
	}
	if err := s.orgs.AddMods(ctx, mods); err != nil {
		return kerrors.WithMsg(err, "Failed to add org mods")
	}

	return nil
}

func (s *Service) userRoleDelete(ctx context.Context, pinger events.Pinger, props role.RolesProps) error {
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
	if err := s.orgs.RmMembers(ctx, props.Userid, memberorgids); err != nil {
		return kerrors.WithMsg(err, "Failed to remove org members")
	}
	if err := s.orgs.RmMods(ctx, props.Userid, modorgids); err != nil {
		return kerrors.WithMsg(err, "Failed to remove org mods")
	}

	return nil
}

func (s *Service) userUpdateHook(ctx context.Context, pinger events.Pinger, props user.UpdateUserProps) error {
	if err := s.orgs.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update member username")
	}
	return nil
}
