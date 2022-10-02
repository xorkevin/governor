package org

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

// Org event kinds
const (
	OrgEventKindDelete = "delete"
)

type (
	orgEventDec struct {
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}

	orgEventEnc struct {
		Kind    string      `json:"kind"`
		Payload interface{} `json:"payload"`
	}

	// OrgEvent is an org event
	OrgEvent struct {
		Kind   string
		Delete DeleteOrgProps
	}

	// DeleteOrgProps are properties of a deleted org
	DeleteOrgProps struct {
		OrgID string `json:"orgid"`
	}

	// HandlerFunc is an org event handler
	HandlerFunc = func(ctx context.Context, props OrgEvent) error

	// Orgs is an organization management service
	Orgs interface {
		GetByID(ctx context.Context, orgid string) (*ResOrg, error)
		GetByName(ctx context.Context, name string) (*ResOrg, error)
		WatchOrgs(group string, opts events.ConsumerOpts, handler, dlqhandler HandlerFunc, maxdeliver int) *events.Watcher
	}

	Service struct {
		orgs        model.Repo
		users       user.Users
		events      events.Events
		ratelimiter ratelimit.Ratelimiter
		gate        gate.Gate
		instance    string
		log         *klog.LevelLogger
		scopens     string
		streamns    string
		streamorgs  string
		streamsize  int64
		eventsize   int32
		wg          *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}

	ctxKeyOrgs struct{}
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
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	return New(orgs, users, ev, ratelimiter, g)
}

// New returns a new Orgs service
func New(orgs model.Repo, users user.Users, ev events.Events, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
	return &Service{
		orgs:        orgs,
		users:       users,
		events:      ev,
		ratelimiter: ratelimiter,
		gate:        g,
		wg:          ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxOrgs(inj, s)
	s.scopens = "gov." + name
	s.streamns = name
	s.streamorgs = name

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
	s.instance = c.Instance

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
	s.wg.Add(1)
	go s.WatchOrgs(s.streamns+".worker", events.ConsumerOpts{}, s.orgEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to orgs stream", nil)

	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.streamorgs, events.StreamOpts{
		Partitions:     16,
		Replicas:       1,
		ReplicaQuorum:  1,
		RetentionAge:   30 * 24 * time.Hour,
		RetentionBytes: int(s.streamsize),
		MaxMsgBytes:    int(s.eventsize),
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

type (
	// ErrorOrgEvent is returned when the org event is malformed
	ErrorOrgEvent struct{}
)

func (e ErrorOrgEvent) Error() string {
	return "Malformed org event"
}

func decodeOrgEvent(msgdata []byte) (*OrgEvent, error) {
	var m orgEventDec
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithKind(err, ErrorOrgEvent{}, "Failed to decode org event")
	}
	props := &OrgEvent{
		Kind: m.Kind,
	}
	switch m.Kind {
	case OrgEventKindDelete:
		if err := kjson.Unmarshal(m.Payload, &props.Delete); err != nil {
			return nil, kerrors.WithKind(err, ErrorOrgEvent{}, "Failed to decode delete org event")
		}
	default:
		return nil, kerrors.WithKind(nil, ErrorOrgEvent{}, "Invalid org event kind")
	}
	return props, nil
}

func encodeOrgEventDelete(props DeleteOrgProps) ([]byte, error) {
	b, err := kjson.Marshal(orgEventEnc{
		Kind:    OrgEventKindDelete,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode delete org props to json")
	}
	return b, nil
}

func (s *Service) WatchOrgs(group string, opts events.ConsumerOpts, handler, dlqhandler HandlerFunc, maxdeliver int) *events.Watcher {
	var dlqfn events.Handler
	if dlqhandler != nil {
		dlqfn = events.HandlerFunc(func(ctx context.Context, msg events.Msg) error {
			props, err := decodeOrgEvent(msg.Value)
			if err != nil {
				return err
			}
			return dlqhandler(ctx, *props)
		})
	}
	return events.NewWatcher(s.events, s.log.Logger, s.streamorgs, group, opts, events.HandlerFunc(func(ctx context.Context, msg events.Msg) error {
		props, err := decodeOrgEvent(msg.Value)
		if err != nil {
			return err
		}
		return handler(ctx, *props)
	}), dlqfn, maxdeliver, s.instance)
}

const (
	roleDeleteBatchSize = 256
)

func (s *Service) orgEventHandler(ctx context.Context, props OrgEvent) error {
	switch props.Kind {
	case OrgEventKindDelete:
		return s.orgEventHandlerDelete(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) orgEventHandlerDelete(ctx context.Context, props DeleteOrgProps) error {
	orgrole := rank.ToOrgName(props.OrgID)
	usrOrgrole := rank.ToUsrName(orgrole)
	modOrgrole := rank.ToModName(orgrole)
	if err := s.users.DeleteRoleInvitations(ctx, modOrgrole); err != nil {
		return kerrors.WithMsg(err, "Failed to delete role mod invitations")
	}
	for {
		userids, err := s.users.GetRoleUsers(ctx, modOrgrole, roleDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get role mods")
		}
		if len(userids) == 0 {
			break
		}
		if err := s.users.DeleteRolesByRole(ctx, modOrgrole, userids); err != nil {
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
		userids, err := s.users.GetRoleUsers(ctx, usrOrgrole, roleDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get role users")
		}
		if len(userids) == 0 {
			break
		}
		if err := s.users.DeleteRolesByRole(ctx, usrOrgrole, userids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete role users")
		}
		if len(userids) < roleDeleteBatchSize {
			break
		}
	}
	return nil
}

func (s *Service) userEventHandler(ctx context.Context, props user.UserEvent) error {
	switch props.Kind {
	case user.UserEventKindUpdate:
		return s.userUpdateEventHandler(ctx, props.Update)
	case user.UserEventKindRoles:
		return s.userRoleEventHandler(ctx, props.Roles)
	default:
		return nil
	}
}

func (s *Service) userRoleEventHandler(ctx context.Context, props user.RolesProps) error {
	if props.Add {
		return s.userCreateRoleEventHandler(ctx, props)
	}
	return s.userDeleteRoleEventHandler(ctx, props)
}

func (s *Service) userCreateRoleEventHandler(ctx context.Context, props user.RolesProps) error {
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

func (s *Service) userDeleteRoleEventHandler(ctx context.Context, props user.RolesProps) error {
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

func (s *Service) userUpdateEventHandler(ctx context.Context, props user.UpdateUserProps) error {
	if err := s.orgs.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update member username")
	}
	return nil
}
