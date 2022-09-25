package eventsapi

import (
	"context"
	"sync/atomic"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
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
		pubsub     pubsub.Pubsub
		config     governor.SecretReader
		log        *klog.LevelLogger
		secretops  chan getSecretOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval time.Duration
		hbmaxfail  int
		done       <-chan struct{}
		apirefresh time.Duration
		apisecret  secretAPI
		aapisecret *atomic.Pointer[secretAPI]
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
		pubsub:     ps,
		secretops:  make(chan getSecretOp),
		ready:      &atomic.Bool{},
		hbfailed:   0,
		aapisecret: &atomic.Pointer[secretAPI]{},
	}
}

func (s *Service) router() *router {
	return &router{
		s: s,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxEventsAPI(inj, s)

	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
	r.SetDefault("apisecret", "")
	r.SetDefault("apirefresh", "1m")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	var err error
	s.hbinterval, err = r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.apirefresh, err = r.GetDuration("apirefresh")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse apirefresh")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"eventsapi.hbinterval": s.hbinterval.String(),
		"eventsapi.hbmaxfail":  s.hbmaxfail,
		"eventsapi.apirefresh": s.apirefresh.String(),
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	sr := s.router()
	sr.mountRoutes(governor.NewMethodRouter(m))
	s.log.Info(ctx, "Mounted http routes", nil)
	return nil
}

func (s *Service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.hbinterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.secretops:
			secret, err := s.handleGetAPISecret(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getSecretRes{
				secret: secret,
				err:    err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *Service) handlePing(ctx context.Context) {
	err := s.refreshAPISecret(ctx)
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
	s.aapisecret.Store(nil)
	s.hbfailed = 0
}

type (
	secretAPI struct {
		Secret string `mapstructure:"secret"`
	}
)

func (s *Service) refreshAPISecret(ctx context.Context) error {
	var apisecret secretAPI
	if err := s.config.GetSecret(ctx, "apisecret", s.apirefresh, &apisecret); err != nil {
		return kerrors.WithMsg(err, "Invalid api secret")
	}
	if apisecret.Secret == "" {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty api secret")
	}
	if apisecret != s.apisecret {
		s.apisecret = apisecret
		s.aapisecret.Store(&apisecret)
		s.log.Info(ctx, "Refreshed api secret with new secret", nil)
	}
	return nil
}

func (s *Service) handleGetAPISecret(ctx context.Context) (string, error) {
	if s.apisecret.Secret == "" {
		if err := s.refreshAPISecret(ctx); err != nil {
			return "", err
		}
	}
	return s.apisecret.Secret, nil
}

func (s *Service) getApiSecret(ctx context.Context) (string, error) {
	if apisecret := s.aapisecret.Load(); apisecret != nil {
		return apisecret.Secret, nil
	}

	res := make(chan getSecretRes)
	op := getSecretOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return "", kerrors.WithMsg(nil, "Events api service shutdown")
	case <-ctx.Done():
		return "", kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.secretops <- op:
		select {
		case <-ctx.Done():
			return "", kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.secret, v.err
		}
	}
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.log.WarnErr(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if !s.ready.Load() {
		return kerrors.WithMsg(nil, "Eventsapi service not ready")
	}
	return nil
}
