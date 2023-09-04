package pubsub

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/ktime"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Msg is a subscription message
	Msg struct {
		Subject string
		Data    []byte
	}

	// Subscription manages an active subscription
	Subscription interface {
		ReadMsg(ctx context.Context) (*Msg, error)
		Close(ctx context.Context) error
		IsClosed() bool
	}

	// Pubsub is an events service with at most once semantics
	Pubsub interface {
		Subscribe(ctx context.Context, subject, group string) (Subscription, error)
		Publish(ctx context.Context, subject string, data []byte) error
	}

	pubsubClient struct {
		client *nats.Conn
		auth   natsauth
	}

	Service struct {
		lc         *lifecycle.Lifecycle[pubsubClient]
		clientname string
		addr       string
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbinterval time.Duration
		hbmaxfail  int
		wg         *ksync.WaitGroup
	}

	subscription struct {
		subject string
		group   string
		log     *klog.LevelLogger
		client  *nats.Conn
		sub     *nats.Subscription
	}
)

// New creates a new pubsub service
func New() *Service {
	return &Service{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
	s.config = r
	s.clientname = r.Config().Instance

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	var err error
	s.hbinterval, err = r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config",
		klog.AString("addr", s.addr),
		klog.AString("hbinterval", s.hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))
	s.lc = lifecycle.New(
		ctx,
		s.handleGetClient,
		s.closeClient,
		s.handlePing,
		s.hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[pubsubClient]) {
	// Check client auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create pubsub client"))
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the pubsub server. This allows vault to be
	// temporarily unavailable without disrupting the client connections.
	username := ""
	if client != nil {
		_, err = client.client.RTT()
		if err == nil {
			s.hbfailed = 0
			return
		}
		username = client.auth.Username
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping pubsub server"),
			klog.AString("addr", s.addr),
			klog.AString("username", username),
		)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to pubsub server"),
		klog.AString("addr", s.addr),
		klog.AString("username", username),
	)
	s.hbfailed = 0
	// first invalidate cached secret in order to ensure that construct client
	// will use refreshed auth
	s.config.InvalidateSecret("auth")
	// must stop the client in order to invalidate cached client, and force wait
	// on newly constructed client
	m.Stop(ctx)
}

// Pubsub client errors
var (
	// ErrConn is returned on a connection error
	ErrConn errConn
	// ErrClient is returned for unknown client errors
	ErrClient errClient
	// ErrClientClosed is returned when the client has been closed
	ErrClientClosed errClientClosed
)

type (
	errConn         struct{}
	errClient       struct{}
	errClientClosed struct{}
)

func (e errConn) Error() string {
	return "Pubsub connection error"
}

func (e errClient) Error() string {
	return "Pubsub client error"
}

func (e errClientClosed) Error() string {
	return "Pubsub client closed"
}

type (
	natsauth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.State[pubsubClient]) (*pubsubClient, error) {
	var auth natsauth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &auth); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if auth.Username == "" {
			return client, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Empty auth")
		}
		if client != nil && auth == client.auth {
			return client, nil
		}
	}

	conn, err := nats.Connect(fmt.Sprintf("nats://%s", s.addr),
		nats.Name(s.clientname),
		nats.UserInfo(auth.Username, auth.Password),
		nats.PingInterval(s.hbinterval),
		nats.MaxPingsOutstanding(s.hbmaxfail),
	)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to connect to pubsub")
	}
	if _, err := conn.RTT(); err != nil {
		conn.Close()
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrConn, "Failed to connect to pubsub")
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to pubsub",
		klog.AString("addr", s.addr),
		klog.AString("username", auth.Username),
	)

	client := &pubsubClient{
		client: conn,
		auth:   auth,
	}
	m.Store(client)

	return client, nil
}

func (s *Service) closeClient(ctx context.Context, client *pubsubClient) {
	if client != nil && !client.client.IsClosed() {
		client.client.Close()
		s.log.Info(ctx, "Closed pubsub connection",
			klog.AString("addr", s.addr),
			klog.AString("username", client.auth.Username),
		)
	}
}

func (s *Service) getClient(ctx context.Context) (*nats.Conn, error) {
	if client := s.lc.Load(ctx); client != nil {
		return client.client, nil
	}

	client, err := s.lc.Construct(ctx)
	if err != nil {
		return nil, err
	}
	return client.client, nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrConn, "Pubsub service not ready")
	}
	return nil
}

// Publish publishes to a subject
func (s *Service) Publish(ctx context.Context, subject string, msgdata []byte) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Publish(subject, msgdata); err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to publish message to subject")
	}
	return nil
}

// Subscribe subscribes to a subject
func (s *Service) Subscribe(ctx context.Context, subject, group string) (Subscription, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	var nsub *nats.Subscription
	if group == "" {
		var err error
		nsub, err = client.SubscribeSync(subject)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrClient, "Failed to create subscription to subject")
		}
	} else {
		var err error
		nsub, err = client.QueueSubscribeSync(subject, group)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrClient, "Failed to create subscription to subject as queue group")
		}
	}
	sub := &subscription{
		subject: subject,
		group:   group,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("subscriber",
			klog.AString("pubsub.subject", subject),
			klog.AString("pubsub.group", group),
		)),
		client: client,
		sub:    nsub,
	}
	sub.log.Info(ctx, "Added subscription")
	return sub, nil
}

func (s *subscription) isClosed() bool {
	return !s.sub.IsValid()
}

// ReadMsg reads a message
func (s *subscription) ReadMsg(ctx context.Context) (*Msg, error) {
	if s.isClosed() {
		return nil, kerrors.WithKind(nil, ErrClientClosed, "Client closed")
	}

	m, err := s.sub.NextMsgWithContext(ctx)
	if err != nil {
		err = kerrors.WithKind(err, ErrClient, "Failed to get message")
		if errors.Is(err, nats.ErrConnectionClosed) {
			return nil, kerrors.WithKind(err, ErrClientClosed, "Client closed")
		}
		return nil, err
	}
	return &Msg{
		Subject: m.Subject,
		Data:    m.Data,
	}, nil
}

// Close closes the subscription
func (s *subscription) Close(ctx context.Context) error {
	if s.isClosed() {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to close subscription to subject")
	}
	s.log.Info(ctx, "Closed subscription")
	return nil
}

func (s *subscription) IsClosed() bool {
	return s.isClosed()
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

type (
	// Handler handles a subscription message
	Handler interface {
		Handle(ctx context.Context, m Msg) error
	}

	// HandlerFunc implements [Handler] for a function
	HandlerFunc func(ctx context.Context, m Msg) error

	// WatchOpts are options for watching a subscription
	WatchOpts struct {
		MinBackoff time.Duration
		MaxBackoff time.Duration
	}

	// Watcher watches over a subscription
	Watcher struct {
		ps      Pubsub
		log     *klog.LevelLogger
		tracer  governor.Tracer
		subject string
		group   string
		handler Handler
	}
)

// Handle implements [Handler]
func (f HandlerFunc) Handle(ctx context.Context, m Msg) error {
	return f(ctx, m)
}

// NewWatcher creates a new watcher
func NewWatcher(ps Pubsub, log klog.Logger, tracer governor.Tracer, subject, group string, handler Handler) *Watcher {
	return &Watcher{
		ps: ps,
		log: klog.NewLevelLogger(log.Sublogger("watcher",
			klog.AString("pubsub.subject", subject),
			klog.AString("pubsub.group", group),
		)),
		tracer:  tracer,
		subject: subject,
		group:   group,
		handler: handler,
	}
}

// Watch watches over a subscription
func (w *Watcher) Watch(ctx context.Context, wg ksync.Waiter, opts WatchOpts) {
	defer wg.Done()

	if opts.MinBackoff == 0 {
		opts.MinBackoff = 1 * time.Second
	}
	if opts.MaxBackoff == 0 {
		opts.MaxBackoff = 15 * time.Second
	}

	delay := opts.MinBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		sub, err := w.ps.Subscribe(ctx, w.subject, w.group)
		if err != nil {
			w.log.Err(ctx, kerrors.WithMsg(err, "Error subscribing"))
			if err := ktime.After(ctx, delay); err != nil {
				continue
			}
			delay = min(delay*2, opts.MaxBackoff)
			continue
		}
		w.consume(ctx, sub, opts)
		delay = opts.MinBackoff
	}
}

func (w *Watcher) consume(ctx context.Context, sub Subscription, opts WatchOpts) {
	defer func() {
		if err := sub.Close(ctx); err != nil {
			w.log.Err(ctx, kerrors.WithMsg(err, "Error closing watched subscription"))
		}
	}()

	delay := opts.MinBackoff
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		m, err := sub.ReadMsg(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if errors.Is(err, ErrClientClosed) {
				return
			}
			w.log.Err(ctx, kerrors.WithMsg(err, "Failed reading message"))
			if err := ktime.After(ctx, delay); err != nil {
				return
			}
			delay = min(delay*2, opts.MaxBackoff)
			continue
		}
		if err := w.consumeMsg(ctx, sub, *m); err != nil {
			if err := ktime.After(ctx, delay); err != nil {
				return
			}
			delay = min(delay*2, opts.MaxBackoff)
			continue
		}
		delay = opts.MinBackoff
	}
}

func (w *Watcher) consumeMsg(ctx context.Context, sub Subscription, m Msg) error {
	ctx = klog.CtxWithAttrs(ctx,
		klog.AString("pubsub.subject", m.Subject),
		klog.AString("pubsub.lreqid", w.tracer.LReqID()),
	)

	start := time.Now()
	if err := w.handler.Handle(ctx, m); err != nil {
		duration := time.Since(start)
		w.log.Err(ctx, kerrors.WithMsg(err, "Failed executing subscription handler"),
			klog.AInt64("duration_ms", duration.Milliseconds()),
		)
		return err
	}
	duration := time.Since(start)
	w.log.Info(ctx, "Handled subscription message",
		klog.AInt64("duration_ms", duration.Milliseconds()),
	)
	return nil
}
