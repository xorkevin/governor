package eventsapi

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/kerrors"
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
		lc         *lifecycle.Lifecycle[secretAPI]
		pubsub     pubsub.Pubsub
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbmaxfail  int
		apirefresh time.Duration
		wg         *ksync.WaitGroup
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
	)
}

// New creates a new events service
func New(ps pubsub.Pubsub) *Service {
	return &Service{
		pubsub:   ps,
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) router() *router {
	return &router{
		s: s,
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxEventsAPI(inj, s)

	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
	r.SetDefault("apisecret", "")
	r.SetDefault("apirefresh", "1m")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.apirefresh, err = r.GetDuration("apirefresh")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse apirefresh")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"eventsapi.hbinterval": hbinterval.String(),
		"eventsapi.hbmaxfail":  s.hbmaxfail,
		"eventsapi.apirefresh": s.apirefresh.String(),
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	s.lc = lifecycle.New(
		ctx,
		s.handleGetAPISecret,
		s.closeAPISecret,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	sr := s.router()
	sr.mountRoutes(governor.NewMethodRouter(m))
	s.log.Info(ctx, "Mounted http routes", nil)
	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[secretAPI]) {
	_, err := m.Construct(ctx)
	if err == nil {
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh api secret"), nil)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"), nil)
	s.hbfailed = 0
	// clear the cached secret because it may now be invalid
	m.Stop(ctx)
}

type (
	secretAPI struct {
		Secret string `mapstructure:"secret"`
	}
)

func (s *Service) handleGetAPISecret(ctx context.Context, m *lifecycle.Manager[secretAPI]) (*secretAPI, error) {
	currentAuth := m.Load(ctx)
	var apisecret secretAPI
	if err := s.config.GetSecret(ctx, "apisecret", s.apirefresh, &apisecret); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid api secret")
	}
	if apisecret.Secret == "" {
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig, "Empty api secret")
	}
	if currentAuth != nil && apisecret == *currentAuth {
		return currentAuth, nil
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Refreshed api secret with new secret", nil)

	m.Store(&apisecret)

	return &apisecret, nil
}

func (s *Service) closeAPISecret(ctx context.Context, secret *secretAPI) {
	// nothing to close
}

func (s *Service) getApiSecret(ctx context.Context) (string, error) {
	if apisecret := s.lc.Load(ctx); apisecret != nil {
		return apisecret.Secret, nil
	}

	apisecret, err := s.lc.Construct(ctx)
	if err != nil {
		return "", err
	}
	return apisecret.Secret, nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithMsg(nil, "Eventsapi service not ready")
	}
	return nil
}
