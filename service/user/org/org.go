package org

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/service/user/role"
)

type (
	// Orgs is an organization management service
	Orgs interface {
	}

	// Service is an Orgs and governor.Service
	Service interface {
		governor.Service
		Orgs
	}

	service struct {
		orgs   model.Repo
		roles  role.Roles
		gate   gate.Gate
		logger governor.Logger
	}

	router struct {
		s service
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
func NewCtx(inj governor.Injector) Service {
	orgs := model.GetCtxRepo(inj)
	roles := role.GetCtxRoles(inj)
	g := gate.GetCtxGate(inj)
	return New(orgs, roles, g)
}

// New returns a new Orgs service
func New(orgs model.Repo, roles role.Roles, g gate.Gate) Service {
	return &service{
		orgs:  orgs,
		roles: roles,
		gate:  g,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxOrgs(inj, s)
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	sr := s.router()
	sr.mountRoute(m)
	l.Info("mounted http routes", nil)

	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.orgs.Setup(); err != nil {
		return err
	}
	l.Info("created userorgs table", nil)

	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
