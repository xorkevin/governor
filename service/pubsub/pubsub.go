package pubsub

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
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
	}

	getClientRes struct {
		client *nats.Conn
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	Service struct {
		client     *nats.Conn
		aclient    *atomic.Pointer[nats.Conn]
		clientname string
		instance   string
		auth       natsauth
		addr       string
		config     governor.SecretReader
		log        *klog.LevelLogger
		ops        chan getOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval time.Duration
		hbmaxfail  int
		done       <-chan struct{}
		reqcount   *atomic.Uint32
	}

	ctxKeyPubsub struct{}

	subscription struct {
		channel string
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
		aclient:  &atomic.Pointer[nats.Conn]{},
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
		reqcount: &atomic.Uint32{},
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
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
	s.instance = c.Instance

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

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	return nil
}

func (s *Service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(s.hbinterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.ops:
			client, err := s.handleGetClient(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getClientRes{
				client: client,
				err:    err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *Service) handlePing(ctx context.Context) {
	var err error
	// Check client auth expiry, and reinit client if about to be expired
	if _, err = s.handleGetClient(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create pubsub client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the pubsub server. This allows vault to be
	// temporarily unavailable without disrupting the client connections.
	if s.client != nil {
		_, err = s.client.RTT()
		if err == nil {
			s.ready.Store(true)
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
	s.aclient.Store(nil)
	s.ready.Store(false)
	s.hbfailed = 0
	s.auth = natsauth{}
	s.config.InvalidateSecret("auth")
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

func (s *Service) handleGetClient(ctx context.Context) (*nats.Conn, error) {
	var secret natsauth
	if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if secret.Password == "" {
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
	}
	if secret == s.auth {
		return s.client, nil
	}

	s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))

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
		return nil, kerrors.WithKind(err, ErrorConn{}, "Failed to connect to pubsub")
	}

	s.client = conn
	s.aclient.Store(s.client)
	s.auth = secret
	s.ready.Store(true)
	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"pubsub.addr": s.addr,
	})
	return s.client, nil
}

func (s *Service) closeClient(ctx context.Context) {
	s.aclient.Store(nil)
	if s.client != nil && !s.client.IsClosed() {
		s.client.Close()
		s.log.Info(ctx, "Closed pubsub connection", klog.Fields{
			"pubsub.addr": s.addr,
		})
	}
	s.client = nil
	s.auth = natsauth{}
}

func (s *Service) getClient(ctx context.Context) (*nats.Conn, error) {
	if client := s.aclient.Load(); client != nil {
		return client, nil
	}

	res := make(chan getClientRes)
	op := getOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return nil, kerrors.WithMsg(nil, "Pubsub service shutdown")
	case <-ctx.Done():
		return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.ops <- op:
		select {
		case <-ctx.Done():
			return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.client, v.err
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
		return kerrors.WithKind(nil, ErrorConn{}, "Pubsub service not ready")
	}
	return nil
}

func (s *Service) lreqID() string {
	return s.instance + "-" + uid.ReqID(s.reqcount.Add(1))
}

// Publish publishes to a channel
func (s *Service) Publish(ctx context.Context, channel string, msgdata []byte) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.Publish(channel, msgdata); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish message to channel")
	}
	return nil
}

// Subscribe subscribes to a channel
func (s *Service) Subscribe(ctx context.Context, channel, group string) (Subscription, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	var nsub *nats.Subscription
	if group == "" {
		var err error
		nsub, err = client.SubscribeSync(channel)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to channel")
		}
	} else {
		var err error
		nsub, err = client.QueueSubscribeSync(channel, group)
		if err != nil {
			return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to channel as queue group")
		}
	}
	sub := &subscription{
		channel: channel,
		group:   group,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("subscriber", klog.Fields{
			"pubsub.channel": channel,
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to close subscription to channel")
	}
	s.log.Info(ctx, "Closed subscription", nil)
	return nil
}

// IsPermanentlyClosed returns if the client is closed
func (s *subscription) IsPermanentlyClosed() bool {
	return s.client.IsClosed()
}
