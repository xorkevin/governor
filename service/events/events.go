package events

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	jsmapi "github.com/nats-io/jsm.go/api"
	jsadvisory "github.com/nats-io/jsm.go/api/jetstream/advisory"
	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
)

type (
	// WorkerFunc is a type alias for a subscriber handler
	WorkerFunc = func(msgdata []byte)

	// StreamWorkerFunc is a type alias for a stream subscriber handler
	StreamWorkerFunc = func(pinger Pinger, msgdata []byte) error

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
		Publish(channel string, msgdata []byte) error
		Subscribe(channel, group string, worker WorkerFunc) (Subscription, error)
		StreamPublish(channel string, msgdata []byte) error
		StreamSubscribe(stream, channel, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error)
		InitStream(name string, subjects []string, opts StreamOpts) error
		DeleteStream(name string) error
		DeleteConsumer(stream, consumer string) error
		DLQSubscribe(targetStream, targetConsumer string, stream, group string, worker StreamWorkerFunc, opts StreamConsumerOpts) (Subscription, error)
	}

	// Service is an Events and governor.Service
	Service interface {
		governor.Service
		Events
	}

	getClientRes struct {
		client *nats.Conn
		stream nats.JetStreamContext
		err    error
	}

	getOp struct {
		res chan<- getClientRes
	}

	subOp struct {
		rm     bool
		sub    *subscription
		stream *streamSubscription
	}

	service struct {
		client          *nats.Conn
		stream          nats.JetStreamContext
		clientname      string
		auth            string
		addr            string
		config          governor.SecretReader
		logger          governor.Logger
		ops             chan getOp
		subops          chan subOp
		subs            map[*subscription]struct{}
		streamSubs      map[*streamSubscription]struct{}
		ready           bool
		canary          <-chan struct{}
		hbinterval      int
		hbmaxfail       int
		minpullduration time.Duration
		done            <-chan struct{}
		apisecret       string
	}

	// Subscription manages an active subscription
	Subscription interface {
		Close() error
	}

	subscription struct {
		s       *service
		channel string
		group   string
		worker  WorkerFunc
		logger  governor.Logger
		sub     *nats.Subscription
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
		Ping() error
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
		ops:        make(chan getOp),
		subops:     make(chan subOp),
		subs:       map[*subscription]struct{}{},
		streamSubs: map[*streamSubscription]struct{}{},
		ready:      false,
		canary:     canary,
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
		return governor.ErrWithMsg(err, "Failed to parse min pull duration")
	}

	apisecret := secretAPI{}
	if err := r.GetSecret("apisecret", 0, &apisecret); err != nil {
		return governor.ErrWithMsg(err, "Invalid api secret")
	}
	s.apisecret = apisecret.Secret

	l.Info("Loaded config", map[string]string{
		"addr":            s.addr,
		"hbinterval":      strconv.Itoa(s.hbinterval),
		"hbmaxfail":       strconv.Itoa(s.hbmaxfail),
		"minpullduration": r.GetStr("minpullduration"),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	if _, _, err := s.getClient(); err != nil {
		return err
	}
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
			s.handlePing()
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
			client, stream, err := s.handleGetClient()
			op.res <- getClientRes{
				client: client,
				stream: stream,
				err:    err,
			}
			close(op.res)
		}
	}
}

func (s *service) handlePing() {
	if s.client != nil {
		select {
		case <-s.canary:
		default:
			s.ready = true
			s.updateSubs(s.client, s.stream)
			return
		}
		s.ready = false
		s.auth = ""
		s.config.InvalidateSecret("auth")
	}
	if _, _, err := s.handleGetClient(); err != nil {
		s.logger.Error("Failed to create events client", map[string]string{
			"error":      err.Error(),
			"actiontype": "createeventsclient",
		})
	}
}

func (s *service) updateSubs(client *nats.Conn, stream nats.JetStreamContext) {
	for k := range s.subs {
		if k.ok() {
			continue
		}
		if err := k.init(client); err != nil {
			s.logger.Error("Failed to subscribe to channel", map[string]string{
				"error":      err.Error(),
				"actiontype": "createeventssuberr",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("Subscribed to channel", map[string]string{
				"actiontype": "createeventssubok",
				"channel":    k.channel,
				"group":      k.group,
			})
		}
	}
	for k := range s.streamSubs {
		if k.ok() {
			continue
		}
		if err := k.init(stream); err != nil {
			s.logger.Error("Failed to subscribe to stream", map[string]string{
				"error":      err.Error(),
				"actiontype": "createeventsstreamsuberr",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("Subscribed to stream", map[string]string{
				"actiontype": "createeventsstreamsubok",
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
			"actiontype": "closeeventssubok",
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
			"actiontype": "closeeventsstreamsubok",
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

func (s *service) handleGetClient() (*nats.Conn, nats.JetStreamContext, error) {
	var secret secretAuth
	if err := s.config.GetSecret("auth", 0, &secret); err != nil {
		return nil, nil, governor.ErrWithMsg(err, "Invalid secret")
	}
	if secret.Password == "" {
		return nil, nil, governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	if secret.Password == s.auth {
		return s.client, s.stream, nil
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
				"actiontype": "pingevents",
			})
		}))
	if err != nil {
		return nil, nil, governor.ErrWithKind(err, ErrConn{}, "Failed to connect to events")
	}
	stream, err := conn.JetStream()
	if err != nil {
		return nil, nil, governor.ErrWithKind(err, ErrClient{}, "Failed to connect to events jetstream")
	}

	s.client = conn
	s.stream = stream
	s.auth = secret.Password
	s.ready = true
	s.canary = canary
	s.logger.Info(fmt.Sprintf("Established connection to %s", s.addr), nil)
	return s.client, s.stream, nil
}

func (s *service) closeClient() {
	if s.client != nil && !s.client.IsClosed() {
		s.client.Close()
		s.logger.Info("Closed events connection", map[string]string{
			"actiontype": "closeeventsok",
			"address":    s.addr,
		})
	}
	s.deinitSubs()
}

func (s *service) getClient() (*nats.Conn, nats.JetStreamContext, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, nil, governor.ErrWithKind(nil, ErrConn{}, "Events service shutdown")
	case s.ops <- op:
		v := <-res
		return v.client, v.stream, v.err
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
		l.Warn("Failed to stop", nil)
	}
}

func (s *service) Health() error {
	if !s.ready {
		return governor.ErrWithKind(nil, ErrConn{}, "Events service not ready")
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
func (s *service) Publish(channel string, msgdata []byte) error {
	client, _, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Publish(channel, msgdata); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to publish message to channel")
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
	sub := &subscription{
		s:       s,
		channel: channel,
		group:   group,
		worker:  worker,
		logger:  l,
	}
	s.addSub(sub, nil)
	return sub, nil
}

func (s *subscription) init(client *nats.Conn) error {
	if s.group == "" {
		sub, err := client.Subscribe(s.channel, s.subscriber)
		if err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to create subscription to channel")
		}
		s.sub = sub
	} else {
		sub, err := client.QueueSubscribe(s.channel, s.group, s.subscriber)
		if err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to create subscription to channel as queue group")
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
	s.worker(msg.Data)
}

// Close closes the subscription
func (s *subscription) Close() error {
	s.s.rmSub(s, nil)
	if s.sub == nil {
		return nil
	}
	if !s.sub.IsValid() {
		return nil
	}
	if err := s.sub.Drain(); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to close subscription to channel")
	}
	return nil
}

// StreamPublish publishes to a stream
func (s *service) StreamPublish(channel string, msgdata []byte) error {
	_, client, err := s.getClient()
	if err != nil {
		return err
	}
	if _, err := client.Publish(channel, msgdata); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to publish message to stream")
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
		return governor.ErrWithKind(err, ErrClient{}, "Failed to create subscription to stream as queue group")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go s.subscriber(ctx, sub, done)
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
					"error": err.Error(),
				})
				return
			}
			continue
		}
		for _, msg := range msgs {
			if err := s.worker(&pinger{msg: msg}, msg.Data); err != nil {
				s.logger.Error("Failed executing worker", map[string]string{
					"error": err.Error(),
				})
			} else {
				if err := msg.Ack(); err != nil {
					s.logger.Error("Failed to ack message", map[string]string{
						"error": err.Error(),
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

func (p *pinger) Ping() error {
	if err := p.msg.InProgress(); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to ping in progress")
	}
	return nil
}

// InitStream initializes a stream
func (s *service) InitStream(name string, subjects []string, opts StreamOpts) error {
	_, client, err := s.getClient()
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
	if _, err := client.StreamInfo(name); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to get stream")
		}
		if _, err := client.AddStream(cfg); err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to create stream")
		}
	} else {
		if _, err := client.UpdateStream(cfg); err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to update stream")
		}
	}
	return nil
}

// DeleteStream deletes a stream
func (s *service) DeleteStream(name string) error {
	_, client, err := s.getClient()
	if err != nil {
		return err
	}
	if _, err := client.StreamInfo(name); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to get stream")
		}
	} else {
		if err := client.DeleteStream(name); err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to delete stream")
		}
	}
	return nil
}

// DeleteConsumer deletes a consumer
func (s *service) DeleteConsumer(stream, consumer string) error {
	_, client, err := s.getClient()
	if err != nil {
		return err
	}
	if _, err := client.ConsumerInfo(stream, consumer); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to get consumer")
		}
	} else {
		if err := client.DeleteConsumer(stream, consumer); err != nil {
			return governor.ErrWithKind(err, ErrClient{}, "Failed to delete consumer")
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
	return s.StreamSubscribe(stream, channelMaxDelivery(targetStream, targetConsumer), group, func(pinger Pinger, msgdata []byte) error {
		schemaType, advmsg, err := jsmapi.ParseMessage(msgdata)
		if err != nil {
			return governor.ErrWithKind(err, ErrInvalidStreamMsg{}, "Failed to parse dead letter queue message with unknown type")
		}
		jse, ok := advmsg.(*jsadvisory.ConsumerDeliveryExceededAdvisoryV1)
		if !ok {
			return governor.ErrWithKind(nil, ErrInvalidStreamMsg{}, fmt.Sprintf("Failed to parse dead letter queue message with type: %s", schemaType))
		}
		if jse.Stream != targetStream || jse.Consumer != targetConsumer {
			return governor.ErrWithKind(nil, ErrInvalidStreamMsg{}, fmt.Sprintf("Invalid target stream and consumer: %s, %s", jse.Stream, jse.Consumer))
		}
		_, client, err := s.getClient()
		if err != nil {
			return err
		}
		msg, err := client.GetMsg(targetStream, jse.StreamSeq)
		if err != nil {
			return governor.ErrWithKind(err, ErrClient{}, fmt.Sprintf("Failed to get msg from stream: %d", jse.StreamSeq))
		}
		return worker(pinger, msg.Data)
	}, opts)
}
