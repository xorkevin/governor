package eventsapi

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/klog"
)

type (
	EventsAPI interface{}

	getSecretRes struct {
		secret string
		err    error
	}

	getSecretOp struct {
		ctx context.Context
		res chan<- getSecretRes
	}

	Service struct {
		pubsub  pubsub.Pubsub
		gate    gate.Gate
		log     *klog.LevelLogger
		scopens string
	}

	router struct {
		s *Service
	}
)

// New creates a new events service
func New(ps pubsub.Pubsub, g gate.Gate) *Service {
	return &Service{
		pubsub: ps,
		gate:   g,
	}
}

func (s *Service) router() *router {
	return &router{
		s: s,
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	s.scopens = "gov." + r.Name()
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))

	sr := s.router()
	sr.mountRoutes(governor.NewMethodRouter(kit.Router))
	s.log.Info(ctx, "Mounted http routes")
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}
