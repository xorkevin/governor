package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	jsmapi "github.com/nats-io/jsm.go/api"
	jsadvisory "github.com/nats-io/jsm.go/api/jetstream/advisory"
	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// WorkerFunc is a type alias for a subscriber handler
	WorkerFunc = func(ctx context.Context, topic string, msgdata []byte) error

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
		instance        string
		auth            secretAuth
		addr            string
		config          governor.SecretReader
		log             *klog.LevelLogger
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
		reqcount        *atomic.Uint32
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
		log     *klog.LevelLogger
		sub     *nats.Subscription
		ctx     context.Context
		cancel  context.CancelFunc
	}

	syncSubscription struct {
		s       *service
		channel string
		group   string
		worker  WorkerFunc
		log     *klog.LevelLogger
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
		log     *klog.LevelLogger
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
		reqcount:   &atomic.Uint32{},
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
	// ErrorConn is returned on a connection error
	ErrorConn struct{}
	// ErrorClient is returned for unknown client errors
	ErrorClient struct{}
	// ErrorInvalidStreamMsg is returned for invalid stream messages
	ErrorInvalidStreamMsg struct{}
)

func (e ErrorConn) Error() string {
	return "Events connection error"
}

func (e ErrorClient) Error() string {
	return "Events client error"
}

func (e ErrorInvalidStreamMsg) Error() string {
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

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r
	s.clientname = c.Hostname + "-" + c.Instance
	s.instance = c.Instance

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")
	var err error
	s.minpullduration, err = time.ParseDuration(r.GetStr("minpullduration"))
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse min pull duration")
	}
	s.apirefresh = r.GetInt("apirefresh")

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"events.addr":            s.addr,
		"events.hbinterval":      s.hbinterval,
		"events.hbmaxfail":       s.hbmaxfail,
		"events.minpullduration": r.GetStr("minpullduration"),
		"events.apirefresh":      s.apirefresh,
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

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))
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
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create events client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then check the canary. This allows vault to be
	// temporarily unavailable without disrupting the client connections. The
	// canary is closed after failing the maximum number of heartbeats already,
	// so no need to track number of heartbeat failures.
	if s.client != nil {
		select {
		case <-s.canary:
			s.aclient.Store(nil)
			s.ready.Store(false)
			s.auth = secretAuth{}
			s.config.InvalidateSecret("auth")
		default:
			s.ready.Store(true)
			s.updateSubs(ctx, s.client)
		}
	}
	err := s.refreshApiSecret(ctx)
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

func (s *service) refreshApiSecret(ctx context.Context) error {
	var apisecret secretAPI
	if err := s.config.GetSecret(ctx, "apisecret", int64(s.apirefresh), &apisecret); err != nil {
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

func (s *service) updateSubs(ctx context.Context, client *natsClient) {
	for k := range s.subs {
		if k.ok() {
			continue
		}
		if err := k.init(client.client); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to subscribe to channel"), klog.Fields{
				"events.channel": k.channel,
				"events.group":   k.group,
			})
		} else {
			s.log.Info(ctx, "Subscribed to channel", klog.Fields{
				"events.channel": k.channel,
				"events.group":   k.group,
			})
		}
	}
	for k := range s.streamSubs {
		if k.ok() {
			continue
		}
		if err := k.init(client.stream); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to subscribe to stream"), klog.Fields{
				"events.channel": k.channel,
				"events.group":   k.group,
			})
		} else {
			s.log.Info(ctx, "Subscribed to stream", klog.Fields{
				"events.channel": k.channel,
				"events.group":   k.group,
			})
		}
	}
}

func (s *service) deinitSubs(ctx context.Context) {
	for k := range s.subs {
		if !k.ok() {
			continue
		}
		k.deinit()
		s.log.Info(ctx, "Closed subscription", klog.Fields{
			"events.channel": k.channel,
			"events.group":   k.group,
		})
	}
	for k := range s.streamSubs {
		if !k.ok() {
			continue
		}
		k.deinit()
		s.log.Info(ctx, "Closed stream subscription", klog.Fields{
			"events.channel": k.channel,
			"events.group":   k.group,
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
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
	}
	if secret == s.auth {
		return s.client, nil
	}

	s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))

	canary := make(chan struct{})
	conn, err := nats.Connect(fmt.Sprintf("nats://%s", s.addr),
		nats.Name(s.clientname),
		nats.Token(secret.Password),
		nats.NoReconnect(),
		nats.PingInterval(time.Duration(s.hbinterval)*time.Second),
		nats.MaxPingsOutstanding(s.hbmaxfail),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			close(canary)
			if err != nil {
				s.log.Err(ctx, kerrors.WithMsg(err, "Lost connection to events"), nil)
			} else {
				s.log.Info(ctx, "Disconnected from events", nil)
			}
		}))
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorConn{}, "Failed to connect to events")
	}
	stream, err := conn.JetStream()
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to connect to events jetstream")
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
	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"events.addr": s.addr,
	})
	return s.client, nil
}

func (s *service) closeClient(ctx context.Context) {
	if s.client != nil && !s.client.client.IsClosed() {
		s.client.client.Close()
		s.log.Info(ctx, "Closed events connection", klog.Fields{
			"events.addr": s.addr,
		})
	}
	s.deinitSubs(klog.ExtendCtx(context.Background(), ctx, nil))
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

func (s *service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.log.WarnErr(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *service) Health(ctx context.Context) error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, ErrorConn{}, "Events service not ready")
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

func (s *service) lreqID() string {
	return s.instance + "-" + uid.ReqID(s.reqcount.Add(1))
}

// Publish publishes to a channel
func (s *service) Publish(ctx context.Context, channel string, msgdata []byte) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if err := client.client.Publish(channel, msgdata); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish message to channel")
	}
	return nil
}

// Subscribe subscribes to a channel
func (s *service) Subscribe(channel, group string, worker WorkerFunc) (Subscription, error) {
	subctx, cancel := context.WithCancel(context.Background())
	subctx = klog.WithFields(subctx, klog.Fields{
		"events.channel": channel,
		"events.group":   group,
	})
	sub := &subscription{
		s:       s,
		channel: channel,
		group:   group,
		worker:  worker,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("subscriber", klog.Fields{
			"events.channel": channel,
			"events.group":   group,
		})),
		ctx:    subctx,
		cancel: cancel,
	}
	s.addSub(sub, nil)
	return sub, nil
}

func (s *subscription) init(client *nats.Conn) error {
	if s.group == "" {
		sub, err := client.Subscribe(s.channel, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to channel")
		}
		s.sub = sub
	} else {
		sub, err := client.QueueSubscribe(s.channel, s.group, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to channel as queue group")
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
	ctx := klog.WithFields(s.ctx, klog.Fields{
		"events.subject": msg.Subject,
		"events.lreqid":  s.s.lreqID(),
	})
	if err := s.worker(ctx, msg.Subject, msg.Data); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed executing worker"), nil)
	}
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to close subscription to channel")
	}
	return nil
}

// SubscribeSync subscribes to a channel synchronously
func (s *service) SubscribeSync(ctx context.Context, channel, group string, worker WorkerFunc) (SyncSubscription, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	subctx, cancel := context.WithCancel(context.Background())
	subctx = klog.WithFields(subctx, klog.Fields{
		"events.channel": channel,
		"events.group":   group,
	})
	sub := &syncSubscription{
		s:       s,
		channel: channel,
		group:   group,
		worker:  worker,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("syncsubscriber", klog.Fields{
			"events.channel": channel,
			"events.group":   group,
		})),
		ctx:    subctx,
		cancel: cancel,
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
			return kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to channel")
		}
		s.sub = sub
	} else {
		sub, err := client.QueueSubscribe(s.channel, s.group, s.subscriber)
		if err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to channel as queue group")
		}
		s.sub = sub
	}
	s.canary = canary
	return nil
}

func (s *syncSubscription) subscriber(msg *nats.Msg) {
	ctx := klog.WithFields(s.ctx, klog.Fields{
		"events.subject": msg.Subject,
		"events.lreqid":  s.s.lreqID(),
	})
	if err := s.worker(ctx, msg.Subject, msg.Data); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed executing worker"), nil)
	}
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to close subscription to channel")
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish message to stream")
	}
	return nil
}

// StreamSubscribe subscribes to a stream
func (s *service) StreamSubscribe(stream, channel, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error) {
	done := make(chan struct{})
	close(done)
	sub := &streamSubscription{
		s:       s,
		stream:  stream,
		channel: channel,
		group:   group,
		opts:    opts,
		worker:  worker,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("streamsubscriber", klog.Fields{
			"events.stream":  stream,
			"events.channel": channel,
			"events.group":   group,
		})),
		done: done,
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to create subscription to stream as queue group")
	}
	subctx, cancel := context.WithCancel(context.Background())
	subctx = klog.WithFields(subctx, klog.Fields{
		"events.stream":  s.stream,
		"events.channel": s.channel,
		"events.group":   s.group,
	})
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
				s.log.Err(ctx, kerrors.WithMsg(err, "Failed obtaining messages"), nil)
				return
			}
			continue
		}
		for _, msg := range msgs {
			msgctx := klog.WithFields(ctx, klog.Fields{
				"events.subject": msg.Subject,
				"events.lreqid":  s.s.lreqID(),
			})
			meta, err := msg.Metadata()
			if err != nil {
				s.log.Err(msgctx, kerrors.WithMsg(err, "Failed getting message metadata"), nil)
			}
			msgctx = klog.WithFields(msgctx, klog.Fields{
				"events.msg.stream":    meta.Stream,
				"events.msg.consumer":  meta.Consumer,
				"events.msg.seqnum":    meta.Sequence.Stream,
				"events.msg.delivered": meta.NumDelivered,
			})
			if err := s.worker(msgctx, &pinger{msg: msg}, msg.Subject, msg.Data); err != nil {
				s.log.Err(msgctx, kerrors.WithMsg(err, "Failed executing worker"), nil)
			} else {
				if err := msg.Ack(); err != nil {
					s.log.Err(msgctx, kerrors.WithMsg(err, "Failed to ack message"), nil)
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to ping in progress")
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
			return kerrors.WithKind(err, ErrorClient{}, "Failed to get stream")
		}
		if _, err := client.stream.AddStream(cfg, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to create stream")
		}
	} else {
		if _, err := client.stream.UpdateStream(cfg, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to update stream")
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
			return kerrors.WithKind(err, ErrorClient{}, "Failed to get stream")
		}
	} else {
		if err := client.stream.DeleteStream(name, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to delete stream")
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
			return kerrors.WithKind(err, ErrorClient{}, "Failed to get consumer")
		}
	} else {
		if err := client.stream.DeleteConsumer(stream, consumer, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to delete consumer")
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
			return kerrors.WithKind(err, ErrorInvalidStreamMsg{}, "Failed to parse dead letter queue message with unknown type")
		}
		jse, ok := advmsg.(*jsadvisory.ConsumerDeliveryExceededAdvisoryV1)
		if !ok {
			return kerrors.WithKind(nil, ErrorInvalidStreamMsg{}, fmt.Sprintf("Failed to parse dead letter queue message with type: %s", schemaType))
		}
		if jse.Stream != targetStream || jse.Consumer != targetConsumer {
			return kerrors.WithKind(nil, ErrorInvalidStreamMsg{}, fmt.Sprintf("Invalid target stream and consumer: %s, %s", jse.Stream, jse.Consumer))
		}
		client, err := s.getClient(ctx)
		if err != nil {
			return err
		}
		msg, err := client.stream.GetMsg(targetStream, jse.StreamSeq, nats.Context(ctx))
		if err != nil {
			return kerrors.WithKind(err, ErrorClient{}, fmt.Sprintf("Failed to get msg from stream: %d", jse.StreamSeq))
		}
		return worker(ctx, pinger, msg.Subject, msg.Data)
	}, opts)
}
