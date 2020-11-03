package org

import (
	"context"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/service/user/role"
)

type (
	Service interface {
		governor.Service
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
)

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
