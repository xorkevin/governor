package conduit

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/chat/model"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// Conduit is a service for messaging
	Conduit interface {
	}

	// Service is the public interface for the conduit service
	Service interface {
		governor.Service
		Conduit
	}

	service struct {
		repo   model.Repo
		gate   gate.Gate
		logger governor.Logger
	}

	router struct {
		s service
	}

	ctxKeyConduit struct{}
)

// GetCtxConduit returns a Conduit service from the context
func GetCtxCourier(inj governor.Injector) Conduit {
	v := inj.Get(ctxKeyConduit{})
	if v == nil {
		return nil
	}
	return v.(Conduit)
}

// setCtxConduit sets a Conduit service in the context
func setCtxConduit(inj governor.Injector, c Conduit) {
	inj.Set(ctxKeyConduit{}, c)
}

// NewCtx creates a new Conduit service from a context
func NewCtx(inj governor.Injector) Service {
	repo := model.GetCtxRepo(inj)
	g := gate.GetCtxGate(inj)
	return New(repo, g)
}

// New creates a new Conduit service
func New(repo model.Repo, g gate.Gate) Service {
	return &service{
		repo: repo,
		gate: g,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxConduit(inj, s)
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.repo.Setup(); err != nil {
		return err
	}
	l.Info("Created conduit chat tables", nil)

	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
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
