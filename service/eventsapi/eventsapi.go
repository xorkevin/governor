package eventsapi

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/klog"
)

type (
	EventsAPI interface {
	}

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

	ctxKeyEventsAPI struct{}
)

// GetCtxEventsAPI returns an EventsAPI from the context
func GetCtxEvents(inj governor.Injector) EventsAPI {
	v := inj.Get(ctxKeyEventsAPI{})
	if v == nil {
		return nil
	}
	return v.(EventsAPI)
}

// setCtxEventsAPI sets an EventsAPI in the context
func setCtxEventsAPI(inj governor.Injector, ea EventsAPI) {
	inj.Set(ctxKeyEventsAPI{}, ea)
}

// NewCtx creates a new EventsAPI service from a context
func NewCtx(inj governor.Injector) *Service {
	return New(
		pubsub.GetCtxPubsub(inj),
		gate.GetCtxGate(inj),
	)
}

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

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxEventsAPI(inj, s)
	s.scopens = "gov." + r.Name()
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	sr := s.router()
	sr.mountRoutes(governor.NewMethodRouter(m))
	s.log.Info(ctx, "Mounted http routes", nil)
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
