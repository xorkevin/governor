package events

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	jsmapi "github.com/nats-io/jsm.go/api"
	jsadvisory "github.com/nats-io/jsm.go/api/jetstream/advisory"
	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
)

type (
	// WorkerFunc is a type alias for a subscriber handler
	WorkerFunc = func(ctx context.Context, topic string, msgdata []byte)

	// StreamWorkerFunc is a type alias for a stream subscriber handler
	StreamWorkerFunc = func(ctx context.Context, pinger Pinger, topic string, msgdata []byte) error

	// StreamOpts are opts for streams
	StreamOpts struct {
		Replicas   int
		MaxAge     time.Duration
		MaxBytes   int64
		MaxMsgSize int32
		MaxMsgs    int64
	}

	// StreamConsumerOpts are opts for stream consumers
	StreamConsumerOpts struct {
		AckWait     time.Duration
		MaxDeliver  int
		MaxPending  int
		MaxRequests int
	}

	// Events is a service wrapper around an event stream client
	Events interface {
		Publish(ctx context.Context, channel string, msgdata []byte) error
		Subscribe(channel, group string, worker WorkerFunc) (Subscription, error)
		SubscribeSync(ctx context.Context, channel, group string, worker WorkerFunc) (SyncSubscription, error)
		StreamPublish(ctx context.Context, channel string, msgdata []byte) error
		StreamSubscribe(stream, channel, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error)
		InitStream(ctx context.Context, name string, subjects []string, opts StreamOpts) error
		DeleteStream(ctx context.Context, name string) error
		DeleteConsumer(ctx context.Context, stream, consumer string) error
		DLQSubscribe(targetStream, targetConsumer string, stream, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error)
	}

	// Service is an Events and governor.Service
	Service interface {
		governor.Service
		Events
	}

	natsClient struct {
		client *nats.Conn
		stream nats.JetStreamContext
		canary <-chan struct{}
	}

	getClientRes struct {
		client *natsClient
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	subOp struct {
		rm     bool
		sub    *subscription
		stream *streamSubscription
	}

	getSecretRes struct {
		secret string
		err    error
	}

	getSecretOp struct {
		ctx context.Context
		res chan<- getSecretRes
	}

	service struct {
		client          *natsClient
		aclient         *atomic.Pointer[natsClient]
		clientname      string
		auth            secretAuth
		addr            string
		config          governor.SecretReader
		logger          governor.Logger
		ops             chan getOp
		subops          chan subOp
		secretops       chan getSecretOp
		subs            map[*subscription]struct{}
		streamSubs      map[*streamSubscription]struct{}
		ready           *atomic.Bool
		canary          <-chan struct{}
		hbfailed        int
		hbinterval      int
		hbmaxfail       int
		minpullduration time.Duration
		done            <-chan struct{}
		apirefresh      int
		apisecret       secretAPI
		aapisecret      *atomic.Pointer[secretAPI]
	}

	router struct {
		s *service
	}

	// Subscription manages an active subscription
	Subscription interface {
		Close() error
	}

	// SyncSubscription manages an active synchronous subscription
	SyncSubscription interface {
		Done() <-chan struct{}
		Close() error
	}

	subscription struct {
		s       *service
		channel string
		group   string
		worker  WorkerFunc
		logger  governor.Logger
		sub     *nats.Subscription
		ctx     context.Context
		cancel  context.CancelFunc
	}

	syncSubscription struct {
		channel string
		group   string
		worker  WorkerFunc
		logger  governor.Logger
		sub     *nats.Subscription
		canary  <-chan struct{}
		ctx     context.Context
		cancel  context.CancelFunc
	}

	streamSubscription struct {
		s       *service
		stream  string
		channel string
		group   string
		opts    StreamConsumerOpts
		worker  StreamWorkerFunc
		logger  governor.Logger
		cancel  context.CancelFunc
		done    <-chan struct{}
	}

	// Pinger pings in progress liveness checks
	Pinger interface {
		Ping(ctx context.Context) error
	}

	pinger struct {
		msg *nats.Msg
	}

	ctxKeyEvents struct{}
)

// GetCtxEvents returns an Events from the context
func GetCtxEvents(inj governor.Injector) Events {
	v := inj.Get(ctxKeyEvents{})
	if v == nil {
		return nil
	}
	return v.(Events)
}

// setCtxEvents sets an Events in the context
func setCtxEvents(inj governor.Injector, p Events) {
	inj.Set(ctxKeyEvents{}, p)
}

// New creates a new events service
func New() Service {
	canary := make(chan struct{})
	close(canary)
	return &service{
		aclient:    &atomic.Pointer[natsClient]{},
		ops:        make(chan getOp),
		subops:     make(chan subOp),
		subs:       map[*subscription]struct{}{},
		streamSubs: map[*streamSubscription]struct{}{},
		ready:      &atomic.Bool{},
		canary:     canary,
		hbfailed:   0,
		aapisecret: &atomic.Pointer[secretAPI]{},
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxEvents(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 3)
	r.SetDefault("minpullduration", "100ms")
	r.SetDefault("apirefresh", 60)
}

type (
	// ErrConn is returned on a connection error
	ErrConn struct{}
	// ErrClient is returned for unknown client errors
	ErrClient struct{}
	// ErrInvalidStreamMsg is returned for invalid stream messages
	ErrInvalidStreamMsg struct{}
)

func (e ErrConn) Error() string {
	return "Events connection error"
}

func (e ErrClient) Error() string {
	return "Events client error"
}

func (e ErrInvalidStreamMsg) Error() string {
	return "Events invalid stream message"
}

func (s *service) router() *router {
	return &router{
		s: s,
	}
}

type (
	secretAPI struct {
		Secret string `mapstructure:"secret"`
	}
)

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r
	s.clientname = c.Hostname

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")
	var err error
	s.minpullduration, err = time.ParseDuration(r.GetStr("minpullduration"))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse min pull duration")
	}
	s.apirefresh = r.GetInt("apirefresh")

	l.Info("Loaded config", map[string]string{
		"addr":            s.addr,
		"hbinterval":      strconv.Itoa(s.hbinterval),
		"hbmaxfail":       strconv.Itoa(s.hbmaxfail),
		"minpullduration": r.GetStr("minpullduration"),
		"apirefresh":      strconv.Itoa(s.apirefresh),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	sr := s.router()
	sr.mountRoutes(m)
	l.Info("Mounted http routes", nil)
	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.secretops:
			secret, err := s.handleGetApiSecret(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getSecretRes{
				secret: secret,
				err:    err,
			}:
				close(op.res)
			}
		case op := <-s.subops:
			if op.rm {
				if op.sub != nil {
					delete(s.subs, op.sub)
				}
				if op.stream != nil {
					delete(s.streamSubs, op.stream)
				}
			} else {
				if op.sub != nil {
					s.subs[op.sub] = struct{}{}
				}
				if op.stream != nil {
					s.streamSubs[op.stream] = struct{}{}
				}
			}
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

func (s *service) handlePing(ctx context.Context) {
	// Check client auth expiry, and reinit client if about to be expired
	if _, err := s.handleGetClient(ctx); err != nil {
		s.logger.Error("Failed to create events client", map[string]string{
			"error":      err.Error(),
			"actiontype": "events_create_client",
		})
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then check the canary. This allows vault to be
	// temporarily unavailable without disrupting the client connections. The
	// canary is closed after failing the maximum number of heartbeats already,
	// so no need to track number of heartbeat failures.
	canaryIsLive := false
	if s.client != nil {
		select {
		case <-s.canary:
			s.aclient.Store(nil)
			s.ready.Store(false)
			s.auth = secretAuth{}
			s.config.InvalidateSecret("auth")
		default:
			canaryIsLive = true
			s.updateSubs(s.client)
		}
	}
	err := s.refreshApiSecret(ctx)
	if err == nil {
		if canaryIsLive {
			s.ready.Store(true)
		}
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.logger.Warn("Failed to refresh api secret", map[string]string{
			"error":      err.Error(),
			"actiontype": "events_refresh_api_secret",
		})
		return
	}
	s.logger.Error("Failed max refresh attempts", map[string]string{
		"error":      err.Error(),
		"actiontype": "events_refresh_api_secret",
	})
	s.aapisecret.Store(nil)
	s.ready.Store(false)
	s.hbfailed = 0
}

func (s *service) refreshApiSecret(ctx context.Context) error {
	var apisecret secretAPI
	if err := s.config.GetSecret(ctx, "apisecret", int64(s.apirefresh), &apisecret); err != nil {
		return kerrors.WithMsg(err, "Invalid api secret")
	}
	if apisecret.Secret == "" {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Empty api secret")
	}
	if apisecret != s.apisecret {
		s.apisecret = apisecret
		s.aapisecret.Store(&apisecret)
		s.logger.Info("Refreshed api secret with new secret", map[string]string{
			"actiontype": "events_refresh_api_secret",
		})
	}
	return nil
}

func (s *service) handleGetApiSecret(ctx context.Context) (string, error) {
	if s.apisecret.Secret == "" {
		if err := s.refreshApiSecret(ctx); err != nil {
			return "", err
		}
	}
	return s.apisecret.Secret, nil
}

func (s *service) getApiSecret(ctx context.Context) (string, error) {
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
		return "", kerrors.WithMsg(nil, "Events service shutdown")
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

func (s *service) updateSubs(client *natsClient) {
	for k := range s.subs {
		if k.ok() {
			continue
		}
		if err := k.init(client.client); err != nil {
			s.logger.Error("Failed to subscribe to channel", map[string]string{
				"error":      err.Error(),
				"actiontype": "events_create_sub",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("Subscribed to channel", map[string]string{
				"actiontype": "events_create_sub_ok",
				"channel":    k.channel,
				"group":      k.group,
			})
		}
	}
	for k := range s.streamSubs {
		if k.ok() {
			continue
		}
		if err := k.init(client.stream); err != nil {
			s.logger.Error("Failed to subscribe to stream", map[string]string{
				"error":      err.Error(),
				"actiontype": "events_create_stream_sub",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("Subscribed to stream", map[string]string{
				"actiontype": "events_create_stream_sub_ok",
				"channel":    k.channel,
				"group":      k.group,
			})
		}
	}
}

func (s *service) deinitSubs() {
	for k := range s.subs {
		if !k.ok() {
			continue
		}
		k.deinit()
		s.logger.Info("Closed subscription", map[string]string{
			"actiontype": "events_close_sub_ok",
			"channel":    k.channel,
			"group":      k.group,
		})
	}
	for k := range s.streamSubs {
		if !k.ok() {
			continue
		}
		k.deinit()
		s.logger.Info("Closed stream subscription", map[string]string{
			"actiontype": "events_close_stream_sub_ok",
			"channel":    k.channel,
			"group":      k.group,
		})
	}
}

type (
	secretAuth struct {
		Password string `mapstructure:"password"`
	}
)

func (s *service) handleGetClient(ctx context.Context) (*natsClient, error) {
	var secret secretAuth
	if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if secret.Password == "" {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "Empty auth")
	}
	if secret == s.auth {
		return s.client, nil
	}

	s.closeClient()

	canary := make(chan struct{})
	conn, err := nats.Connect(fmt.Sprintf("nats://%s", s.addr),
		nats.Name(s.clientname),
		nats.Token(secret.Password),
		nats.NoReconnect(),
		nats.PingInterval(time.Duration(s.hbinterval)*time.Second),
		nats.MaxPingsOutstanding(s.hbmaxfail),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			close(canary)
			errmsg := "nil err"
			if err != nil {
				errmsg = err.Error()
			}
			s.logger.Error("Lost connection to events", map[string]string{
				"error":      errmsg,
				"actiontype": "events_ping",
			})
		}))
	if err != nil {
		return nil, kerrors.WithKind(err, ErrConn{}, "Failed to connect to events")
	}
	stream, err := conn.JetStream()
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient{}, "Failed to connect to events jetstream")
	}

	s.client = &natsClient{
		client: conn,
		stream: stream,
		canary: canary,
	}
	s.aclient.Store(s.client)
	s.auth = secret
	s.ready.Store(true)
	s.canary = canary
	s.logger.Info("Established connection to event stream", map[string]string{
		"addr": s.addr,
	})
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client != nil && !s.client.client.IsClosed() {
		s.client.client.Close()
		s.logger.Info("Closed events connection", map[string]string{
			"actiontype": "events_close_ok",
			"address":    s.addr,
		})
	}
	s.deinitSubs()
}

func (s *service) getClient(ctx context.Context) (*natsClient, error) {
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
		return nil, kerrors.WithMsg(nil, "Events service shutdown")
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

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("Failed to stop", map[string]string{
			"error":      ctx.Err().Error(),
			"actiontype": "events_stop",
		})
	}
}

func (s *service) Health() error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, ErrConn{}, "Events service not ready")
	}
	return nil
}

func (s *service) addSub(sub *subscription, stream *streamSubscription) {
	op := subOp{
		rm:     false,
		sub:    sub,
		stream: stream,
	}
	select {
	case <-s.done:
	case s.subops <- op:
	}
}

func (s *service) rmSub(sub *subscription, stream *streamSubscription) {
	op := subOp{
		rm:     true,
		sub:    sub,
		stream: stream,
	}
	select {
	case <-s.done:
	case s.subops <- op:
	}
}

// Publish publishes to a channel
func (s *service) Publish(ctx context.Context, channel string, msgdata []byte) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.client.Publish(channel, msgdata); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to publish message to channel")
	}
	return nil
}

// Subscribe subscribes to a channel
func (s *service) Subscribe(channel, group string, worker WorkerFunc) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": channel,
		"group":   group,
	})
	subctx, cancel := context.WithCancel(context.Background())
	sub := &subscription{
		s:       s,
		channel: channel,
		group:   group,
		worker:  worker,
		logger:  l,
		ctx:     subctx,
		cancel:  cancel,
	}
	s.addSub(sub, nil)
	return sub, nil
}

func (s *subscription) init(client *nats.Conn) error {
	if s.group == "" {
		sub, err := client.Subscribe(s.channel, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to create subscription to channel")
		}
		s.sub = sub
	} else {
		sub, err := client.QueueSubscribe(s.channel, s.group, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to create subscription to channel as queue group")
		}
		s.sub = sub
	}
	return nil
}

func (s *subscription) deinit() {
	s.sub = nil
}

func (s *subscription) ok() bool {
	return s.sub != nil
}

func (s *subscription) subscriber(msg *nats.Msg) {
	s.worker(s.ctx, msg.Subject, msg.Data)
}

// Close closes the subscription
func (s *subscription) Close() error {
	s.s.rmSub(s, nil)
	s.cancel()
	if s.sub == nil {
		return nil
	}
	if !s.sub.IsValid() {
		return nil
	}
	if err := s.sub.Drain(); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to close subscription to channel")
	}
	return nil
}

// SubscribeSync subscribes to a channel synchronously
func (s *service) SubscribeSync(ctx context.Context, channel, group string, worker WorkerFunc) (SyncSubscription, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": channel,
		"group":   group,
	})
	subctx, cancel := context.WithCancel(context.Background())
	sub := &syncSubscription{
		channel: channel,
		group:   group,
		worker:  worker,
		logger:  l,
		ctx:     subctx,
		cancel:  cancel,
	}
	if err := sub.init(client.client, client.canary); err != nil {
		return nil, err
	}
	return sub, nil
}

func (s *syncSubscription) init(client *nats.Conn, canary <-chan struct{}) error {
	if s.group == "" {
		sub, err := client.Subscribe(s.channel, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to create subscription to channel")
		}
		s.sub = sub
	} else {
		sub, err := client.QueueSubscribe(s.channel, s.group, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to create subscription to channel as queue group")
		}
		s.sub = sub
	}
	s.canary = canary
	return nil
}

func (s *syncSubscription) subscriber(msg *nats.Msg) {
	s.worker(s.ctx, msg.Subject, msg.Data)
}

// Done closes the subscription
func (s *syncSubscription) Done() <-chan struct{} {
	return s.canary
}

// Close closes the subscription
func (s *syncSubscription) Close() error {
	s.cancel()
	if !s.sub.IsValid() {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to close subscription to channel")
	}
	return nil
}

// StreamPublish publishes to a stream
func (s *service) StreamPublish(ctx context.Context, channel string, msgdata []byte) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.stream.Publish(channel, msgdata, nats.Context(ctx)); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to publish message to stream")
	}
	return nil
}

// StreamSubscribe subscribes to a stream
func (s *service) StreamSubscribe(stream, channel, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"stream":  stream,
		"channel": channel,
		"group":   group,
	})
	done := make(chan struct{})
	close(done)
	sub := &streamSubscription{
		s:       s,
		stream:  stream,
		channel: channel,
		group:   group,
		opts:    opts,
		worker:  worker,
		logger:  l,
		done:    done,
	}
	s.addSub(nil, sub)
	return sub, nil
}

func (s *streamSubscription) init(client nats.JetStream) error {
	args := make([]nats.SubOpt, 0, 8)
	args = append(args, nats.BindStream(s.stream), nats.ManualAck(), nats.AckExplicit(), nats.DeliverAll())
	if s.opts.AckWait > 0 {
		args = append(args, nats.AckWait(s.opts.AckWait))
	}
	if s.opts.MaxDeliver > 0 {
		args = append(args, nats.MaxDeliver(s.opts.MaxDeliver))
	}
	if s.opts.MaxPending > 0 {
		args = append(args, nats.MaxAckPending(s.opts.MaxPending))
	}
	if s.opts.MaxRequests > 0 {
		args = append(args, nats.PullMaxWaiting(s.opts.MaxRequests))
	}
	sub, err := client.PullSubscribe(
		s.channel,
		s.group,
		args...,
	)
	if err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to create subscription to stream as queue group")
	}
	subctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go s.subscriber(subctx, sub, done)
	s.cancel = cancel
	s.done = done
	return nil
}

func (s *streamSubscription) deinit() {
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
}

func (s *streamSubscription) ok() bool {
	select {
	case <-s.done:
		return false
	default:
		return true
	}
}

func (s *streamSubscription) fetch(ctx context.Context, sub *nats.Subscription) ([]*nats.Msg, error) {
	reqctx, reqcancel := context.WithTimeout(ctx, 5*time.Second)
	defer reqcancel()
	return sub.Fetch(1, nats.Context(reqctx))
}

func (s *streamSubscription) subscriber(ctx context.Context, sub *nats.Subscription, done chan<- struct{}) {
	defer close(done)
	start := time.Now()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msgs, err := s.fetch(ctx, sub)
		if err != nil {
			if !errors.Is(err, context.DeadlineExceeded) {
				s.logger.Error("Failed obtaining messages", map[string]string{
					"error":      err.Error(),
					"actiontype": "events_fetch_stream_msgs",
				})
				return
			}
			continue
		}
		for _, msg := range msgs {
			if err := s.worker(ctx, &pinger{msg: msg}, msg.Subject, msg.Data); err != nil {
				s.logger.Error("Failed executing worker", map[string]string{
					"error":      err.Error(),
					"actiontype": "events_exec_stream_worker",
				})
			} else {
				if err := msg.Ack(); err != nil {
					s.logger.Error("Failed to ack message", map[string]string{
						"error":      err.Error(),
						"actiontype": "events_ack_stream_msg",
					})
				}
			}
		}
		now := time.Now()
		delta := s.s.minpullduration - now.Sub(start)
		start = now
		if delta > 0 {
			time.Sleep(delta)
		}
	}
}

// Close closes the subscription
func (s *streamSubscription) Close() error {
	s.s.rmSub(nil, s)
	if s.cancel != nil {
		s.cancel()
	}
	<-s.done
	return nil
}

func (p *pinger) Ping(ctx context.Context) error {
	if err := p.msg.InProgress(nats.Context(ctx)); err != nil {
		return kerrors.WithKind(err, ErrClient{}, "Failed to ping in progress")
	}
	return nil
}

// InitStream initializes a stream
func (s *service) InitStream(ctx context.Context, name string, subjects []string, opts StreamOpts) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	cfg := &nats.StreamConfig{
		Name:       name,
		Subjects:   subjects,
		Retention:  nats.LimitsPolicy,
		Discard:    nats.DiscardOld,
		Storage:    nats.FileStorage,
		Replicas:   opts.Replicas,
		MaxAge:     opts.MaxAge,
		MaxBytes:   opts.MaxBytes,
		MaxMsgSize: opts.MaxMsgSize,
		MaxMsgs:    opts.MaxMsgs,
	}
	if _, err := client.stream.StreamInfo(name, nats.Context(ctx)); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return kerrors.WithKind(err, ErrClient{}, "Failed to get stream")
		}
		if _, err := client.stream.AddStream(cfg, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to create stream")
		}
	} else {
		if _, err := client.stream.UpdateStream(cfg, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to update stream")
		}
	}
	return nil
}

// DeleteStream deletes a stream
func (s *service) DeleteStream(ctx context.Context, name string) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.stream.StreamInfo(name, nats.Context(ctx)); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return kerrors.WithKind(err, ErrClient{}, "Failed to get stream")
		}
	} else {
		if err := client.stream.DeleteStream(name, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to delete stream")
		}
	}
	return nil
}

// DeleteConsumer deletes a consumer
func (s *service) DeleteConsumer(ctx context.Context, stream, consumer string) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.stream.ConsumerInfo(stream, consumer, nats.Context(ctx)); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return kerrors.WithKind(err, ErrClient{}, "Failed to get consumer")
		}
	} else {
		if err := client.stream.DeleteConsumer(stream, consumer, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrClient{}, "Failed to delete consumer")
		}
	}
	return nil
}

// channelMaxDelivery returns the max delivery error channel
func channelMaxDelivery(stream, consumer string) string {
	return fmt.Sprintf("%s.%s.%s", jsmapi.JSAdvisoryConsumerMaxDeliveryExceedPre, stream, consumer)
}

// DLQSubscribe subscribes to the deadletter queue of another stream consumer
func (s *service) DLQSubscribe(targetStream, targetConsumer string, stream, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error) {
	return s.StreamSubscribe(stream, channelMaxDelivery(targetStream, targetConsumer), group, func(ctx context.Context, pinger Pinger, topic string, msgdata []byte) error {
		schemaType, advmsg, err := jsmapi.ParseMessage(msgdata)
		if err != nil {
			return kerrors.WithKind(err, ErrInvalidStreamMsg{}, "Failed to parse dead letter queue message with unknown type")
		}
		jse, ok := advmsg.(*jsadvisory.ConsumerDeliveryExceededAdvisoryV1)
		if !ok {
			return kerrors.WithKind(nil, ErrInvalidStreamMsg{}, fmt.Sprintf("Failed to parse dead letter queue message with type: %s", schemaType))
		}
		if jse.Stream != targetStream || jse.Consumer != targetConsumer {
			return kerrors.WithKind(nil, ErrInvalidStreamMsg{}, fmt.Sprintf("Invalid target stream and consumer: %s, %s", jse.Stream, jse.Consumer))
		}
		client, err := s.getClient(ctx)
		if err != nil {
			return err
		}
		msg, err := client.stream.GetMsg(targetStream, jse.StreamSeq, nats.Context(ctx))
		if err != nil {
			return kerrors.WithKind(err, ErrClient{}, fmt.Sprintf("Failed to get msg from stream: %d", jse.StreamSeq))
		}
		return worker(ctx, pinger, msg.Subject, msg.Data)
	}, opts)
}
