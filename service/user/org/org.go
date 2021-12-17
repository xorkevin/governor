package org

import (
	"context"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
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
		orgs       model.Repo
		roles      role.Roles
		events     events.Events
		gate       gate.Gate
		logger     governor.Logger
		scopens    string
		streamns   string
		opts       Opts
		streamsize int64
		eventsize  int32
	}

	router struct {
		s *service
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
	ev := events.GetCtxEvents(inj)
	g := gate.GetCtxGate(inj)
	return New(orgs, roles, ev, g)
}

// New returns a new Orgs service
func New(orgs model.Repo, roles role.Roles, ev events.Events, g gate.Gate) Service {
	return &service{
		orgs:   orgs,
		roles:  roles,
		events: ev,
		gate:   g,
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
		s: s,
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
func (s *service) OrgRoleDeleteHook(pinger events.Pinger, msgdata []byte) error {
	props, err := DecodeDeleteOrgProps(msgdata)
	if err != nil {
		return err
	}
	orgrole := rank.ToOrgName(props.OrgID)
	usrOrgrole := rank.ToUsrName(orgrole)
	modOrgrole := rank.ToModName(orgrole)
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
	return nil
}
