package pubsub

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/governor/util/uid"
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
		IsPermanentlyClosed() bool
	}

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
		auth       natsauth
		addr       string
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbinterval time.Duration
		hbmaxfail  int
		wg         *ksync.WaitGroup
	}

	ctxKeyPubsub struct{}

	subscription struct {
		subject string
		group   string
		log     *klog.LevelLogger
		client  *nats.Conn
		sub     *nats.Subscription
	}
)

// GetCtxPubsub returns a Pubsub from the context
func GetCtxPubsub(inj governor.Injector) Pubsub {
	v := inj.Get(ctxKeyPubsub{})
	if v == nil {
		return nil
	}
	return v.(Pubsub)
}

// setCtxPubsub sets a Pubsub in the context
func setCtxPubsub(inj governor.Injector, p Pubsub) {
	inj.Set(ctxKeyPubsub{}, p)
}

// New creates a new pubsub service
func New() *Service {
	return &Service{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxPubsub(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r
	s.clientname = c.Hostname + "-" + c.Instance

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	var err error
	s.hbinterval, err = r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"pubsub.addr":       s.addr,
		"pubsub.hbinterval": s.hbinterval.String(),
		"pubsub.hbmaxfail":  s.hbmaxfail,
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create pubsub client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the pubsub server. This allows vault to be
	// temporarily unavailable without disrupting the client connections.
	if client != nil {
		_, err = client.client.RTT()
		if err == nil {
			s.hbfailed = 0
			return
		}
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping pubsub server"), klog.Fields{
			"pubsub.addr": s.addr,
		})
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to pubusub server"), klog.Fields{
		"pubsub.addr": s.addr,
	})
	s.hbfailed = 0
	// first invalidate cached secret in order to ensure that construct client
	// will use refreshed auth
	s.config.InvalidateSecret("auth")
	// must stop the client in order to invalidate cached client, and force wait
	// on newly constructed client
	m.Stop(ctx)
}

type (
	// ErrorConn is returned on a connection error
	ErrorConn struct{}
	// ErrorClient is returned for unknown client errors
	ErrorClient struct{}
)

func (e ErrorConn) Error() string {
	return "Pubsub connection error"
}

func (e ErrorClient) Error() string {
	return "Pubsub client error"
}

type (
	natsauth struct {
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.Manager[pubsubClient]) (*pubsubClient, error) {
	var secret natsauth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if secret.Password == "" {
			return client, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
		}
		if secret == s.auth {
			return client, nil
		}
	}

	conn, err := nats.Connect(fmt.Sprintf("nats://%s", s.addr),
		nats.Name(s.clientname),
		nats.Token(secret.Password),
		nats.PingInterval(s.hbinterval),
		nats.MaxPingsOutstanding(s.hbmaxfail),
	)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to connect to pubsub")
	}
	if _, err := conn.RTT(); err != nil {
		conn.Close()
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrorConn{}, "Failed to connect to pubsub")
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"pubsub.addr": s.addr,
	})

	client := &pubsubClient{
		client: conn,
		auth:   secret,
	}
	m.Store(client)

	return client, nil
}

func (s *Service) closeClient(ctx context.Context, client *pubsubClient) {
	if client != nil && !client.client.IsClosed() {
		client.client.Close()
		s.log.Info(ctx, "Closed pubsub connection", klog.Fields{
			"pubsub.addr": s.addr,
		})
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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrorConn{}, "Pubsub service not ready")
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish message to subject")
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
			return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to subject")
		}
	} else {
		var err error
		nsub, err = client.QueueSubscribeSync(subject, group)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to subject as queue group")
		}
	}
	sub := &subscription{
		subject: subject,
		group:   group,
		log: klog.NewLevelLogger(klog.Sub(s.log.Logger, "subscriber", klog.Fields{
			"pubsub.subject": subject,
			"pubsub.group":   group,
		})),
		client: client,
		sub:    nsub,
	}
	sub.log.Info(ctx, "Added subscription", nil)
	return sub, nil
}

// ReadMsg reads a message
func (s *subscription) ReadMsg(ctx context.Context) (*Msg, error) {
	m, err := s.sub.NextMsgWithContext(ctx)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to get message")
	}
	return &Msg{
		Subject: m.Subject,
		Data:    m.Data,
	}, nil
}

// Close closes the subscription
func (s *subscription) Close(ctx context.Context) error {
	if !s.sub.IsValid() {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to close subscription to subject")
	}
	s.log.Info(ctx, "Closed subscription", nil)
	return nil
}

// IsPermanentlyClosed returns if the client is closed
func (s *subscription) IsPermanentlyClosed() bool {
	return s.client.IsClosed()
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
		MaxBackoff time.Duration
	}

	// Watcher watches over a subscription
	Watcher struct {
		ps          Pubsub
		log         *klog.LevelLogger
		subject     string
		group       string
		handler     Handler
		reqidprefix string
		reqcount    *atomic.Uint32
	}
)

// Handle implements [Handler]
func (f HandlerFunc) Handle(ctx context.Context, m Msg) error {
	return f(ctx, m)
}

// NewWatcher creates a new watcher
func NewWatcher(ps Pubsub, log klog.Logger, subject, group string, handler Handler, reqidprefix string) *Watcher {
	return &Watcher{
		ps: ps,
		log: klog.NewLevelLogger(klog.Sub(log, "watcher", klog.Fields{
			"pubsub.subject": subject,
			"pubsub.group":   group,
		})),
		subject:     subject,
		group:       group,
		handler:     handler,
		reqidprefix: reqidprefix,
		reqcount:    &atomic.Uint32{},
	}
}

func (w *Watcher) lreqID() string {
	return w.reqidprefix + "-" + uid.ReqID(w.reqcount.Add(1))
}

const (
	watchStartDelay = 1 * time.Second
)

// Watch watches over a subscription
func (w *Watcher) Watch(ctx context.Context, wg ksync.Waiter, opts WatchOpts) {
	defer wg.Done()

	if opts.MaxBackoff == 0 {
		opts.MaxBackoff = 15 * time.Second
	}

	delay := watchStartDelay
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		func() {
			sub, err := w.ps.Subscribe(ctx, w.subject, w.group)
			if err != nil {
				w.log.Err(ctx, kerrors.WithMsg(err, "Error subscribing"), nil)
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
					delay = min(delay*2, opts.MaxBackoff)
					return
				}
			}
			defer func() {
				if err := sub.Close(ctx); err != nil {
					w.log.Err(ctx, kerrors.WithMsg(err, "Error closing watched subscription"), nil)
				}
			}()
			delay = watchStartDelay

			for {
				m, err := sub.ReadMsg(ctx)
				if err != nil {
					if sub.IsPermanentlyClosed() {
						return
					}
					w.log.Err(ctx, kerrors.WithMsg(err, "Failed reading message"), nil)
					select {
					case <-ctx.Done():
						return
					case <-time.After(delay):
						delay = min(delay*2, opts.MaxBackoff)
						continue
					}
				}
				msgctx := klog.WithFields(ctx, klog.Fields{
					"pubsub.subject": m.Subject,
					"pubsub.lreqid":  w.lreqID(),
				})
				start := time.Now()
				if err := w.handler.Handle(msgctx, *m); err != nil {
					duration := time.Since(start)
					w.log.Err(msgctx, kerrors.WithMsg(err, "Failed executing subscription handler"), klog.Fields{
						"pubsub.duration_ms": duration.Milliseconds(),
					})
					select {
					case <-msgctx.Done():
						return
					case <-time.After(delay):
						delay = min(delay*2, opts.MaxBackoff)
						continue
					}
				}
				duration := time.Since(start)
				w.log.Info(msgctx, "Handled subscription message", klog.Fields{
					"pubsub.duration_ms": duration.Milliseconds(),
				})
				delay = watchStartDelay
			}
		}()
	}
}
