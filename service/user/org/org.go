package org

import (
	"context"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/service/user/role"
)

type (
	// Org is an organization management service
	Org interface {
	}

	Service interface {
		governor.Service
		Org
	}

	service struct {
		orgs   orgmodel.Repo
		roles  role.Role
		gate   gate.Gate
		logger governor.Logger
	}

	router struct {
		s service
	}

	ctxKeyOrg struct{}
)

// GetCtxOrg returns an Org service from the context
func GetCtxOrg(ctx context.Context) (Org, error) {
	v := ctx.Value(ctxKeyOrg{})
	if v == nil {
		return nil, governor.NewError("User service not found in context", http.StatusInternalServerError, nil)
	}
	return v.(Org), nil
}

// SetCtxOrg sets an Org service in the context
func SetCtxOrg(ctx context.Context, o Org) context.Context {
	return context.WithValue(ctx, ctxKeyOrg{}, o)
}

// NewCtx creates a new Org service from a context
func NewCtx(ctx context.Context) (Service, error) {
	orgs, err := orgmodel.GetCtxRepo(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := role.GetCtxRole(ctx)
	if err != nil {
		return nil, err
	}
	g, err := gate.GetCtxGate(ctx)
	if err != nil {
		return nil, err
	}
	return New(orgs, roles, g), nil
}

// New returns a new org service
func New(orgs orgmodel.Repo, roles role.Role, g gate.Gate) Service {
	return &service{
		orgs:  orgs,
		roles: roles,
		gate:  g,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
