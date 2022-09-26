package events

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// StreamOpts are opts for streams
	StreamOpts struct {
		Replicas   int
		MaxAge     time.Duration
		MaxBytes   int64
		MaxMsgSize int32
		MaxMsgs    int64
	}

	// ConsumerOpts are opts for event stream consumers
	ConsumerOpts struct {
	}

	// Msg is a subscription message
	Msg struct {
		Topic     string
		Key       string
		Data      []byte
		Partition int
		Offset    int64
		Time      time.Time
		msg       kafka.Message
	}

	// Subscription manages an active subscription
	Subscription interface {
		Close() error
	}

	// Events is a service wrapper around an event stream client
	Events interface {
		Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error)
		Publish(ctx context.Context, topic string, key string, data []byte) error
		InitStream(ctx context.Context, topic string, opts StreamOpts) error
		DeleteStream(ctx context.Context, topic string) error
	}

	kafkaClient struct {
		client *nats.Conn
		stream nats.JetStreamContext
	}

	getClientRes struct {
		client *kafkaClient
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getClientRes
	}

	Service struct {
		client     *kafkaClient
		aclient    *atomic.Pointer[kafkaClient]
		clientname string
		instance   string
		auth       secretAuth
		addr       string
		config     governor.SecretReader
		log        *klog.LevelLogger
		ops        chan getOp
		ready      *atomic.Bool
		hbfailed   int
		hbinterval time.Duration
		hbmaxfail  int
		done       <-chan struct{}
	}

	subscription struct {
		s      *Service
		topic  string
		group  string
		log    *klog.LevelLogger
		reader *kafka.Reader
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
func New() *Service {
	return &Service{
		aclient:  &atomic.Pointer[kafkaClient]{},
		ops:      make(chan getOp),
		ready:    &atomic.Bool{},
		hbfailed: 0,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxEvents(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
}

type (
	// ErrorConn is returned on a connection error
	ErrorConn struct{}
	// ErrorClient is returned for unknown client errors
	ErrorClient struct{}
)

func (e ErrorConn) Error() string {
	return "Events connection error"
}

func (e ErrorClient) Error() string {
	return "Events client error"
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
		"events.addr":       s.addr,
		"events.hbinterval": s.hbinterval.String(),
		"events.hbmaxfail":  s.hbmaxfail,
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
		}
	}
}

type (
	secretAuth struct {
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context) (*kafkaClient, error) {
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
		nats.PingInterval(s.hbinterval),
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

	s.client = &kafkaClient{
		client: conn,
		stream: stream,
	}
	s.aclient.Store(s.client)
	s.auth = secret
	s.ready.Store(true)
	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"events.addr": s.addr,
	})
	return s.client, nil
}

func (s *Service) closeClient(ctx context.Context) {
	if s.client != nil && !s.client.client.IsClosed() {
		s.client.client.Close()
		s.log.Info(ctx, "Closed events connection", klog.Fields{
			"events.addr": s.addr,
		})
	}
}

func (s *Service) getClient(ctx context.Context) (*kafkaClient, error) {
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
		return kerrors.WithKind(nil, ErrorConn{}, "Events service not ready")
	}
	return nil
}

// Publish publishes an event
func (s *Service) Publish(ctx context.Context, topic string, key string, data []byte) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.stream.Publish(topic, data, nats.Context(ctx)); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish message to stream")
	}
	return nil
}

// Subscribe subscribes to an event stream
func (s *Service) Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error) {
	mechanism, err := scram.Mechanism(scram.SHA512, "username", "password")
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create scram mechanism")
	}
	dialer := &kafka.Dialer{
		ClientID: s.clientname + "-" + s.instance,
		DialFunc: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 5 * time.Second,
		}).DialContext,
		SASLMechanism: mechanism,
	}
	readerconfig := kafka.ReaderConfig{
		Brokers: []string{"localhost:9092", "localhost:9093", "localhost:9094"},
		GroupID: group,
		Topic:   topic,
		Dialer:  dialer,
		// message read config
		QueueCapacity:  128,                 // number of messages in queue
		MinBytes:       1,                   // minimum size of batch of messages
		MaxBytes:       1 << 20,             // 1MB, maximum size of batch of messages, maximum single message size
		MaxWait:        5 * time.Second,     // wait for messages for this long before repolling
		StartOffset:    kafka.LastOffset,    // start at latest message
		CommitInterval: 0,                   // synchronously commit offsets
		IsolationLevel: kafka.ReadCommitted, // for transactions, read only committed messages
		// liveness config
		HeartbeatInterval: 5 * time.Second,  // heartbeat the group coordinator at this interval
		SessionTimeout:    15 * time.Second, // declare consumer as dead if no heartbeat within this interval
		// partition config
		// partition assignment strategies
		GroupBalancers: []kafka.GroupBalancer{
			kafka.RangeGroupBalancer{},
			kafka.RoundRobinGroupBalancer{},
		},
		PartitionWatchInterval: 5 * time.Second,    // watch for partition changes on this interval
		WatchPartitionChanges:  true,               // watch for partition changes
		RebalanceTimeout:       30 * time.Second,   // amount of time for group coordinator to wait before finishing rebalancing
		JoinGroupBackoff:       5 * time.Second,    // amount of time to wait before rejoining consumer group after error
		RetentionTime:          7 * 24 * time.Hour, // how long to save consumer group offsets
		// connection config
		ReadBackoffMin:        250 * time.Millisecond, // minimum amount of time to poll for new messages
		ReadBackoffMax:        5 * time.Second,        // maximum amount of time to poll for new messages
		MaxAttempts:           3,                      // number of attempts before reporting errors for establishing connection
		OffsetOutOfRangeError: true,                   // will be permanently true in the future
	}
	if err := readerconfig.Validate(); err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Invalid reader config")
	}
	sub := &subscription{
		s:     s,
		topic: topic,
		group: group,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("subscriber", klog.Fields{
			"events.topic": topic,
			"events.group": group,
		})),
		reader: kafka.NewReader(readerconfig),
	}
	sub.log.Info(ctx, "Added subscription", nil)
	return sub, nil
}

// ReadMsg reads a message
func (s *subscription) ReadMsg(ctx context.Context) (*Msg, error) {
	m, err := s.reader.FetchMessage(ctx)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to read message")
	}
	return &Msg{
		Topic:     m.Topic,
		Key:       string(m.Key),
		Data:      m.Value,
		Partition: m.Partition,
		Offset:    m.Offset,
		Time:      m.Time,
		msg:       m,
	}, nil
}

// Commit commits a new message offset
func (s *subscription) Commit(ctx context.Context, msg Msg) error {
	if err := s.reader.CommitMessages(ctx, msg.msg); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to commit message offset")
	}
	return nil
}

// Close closes the subscription
func (s *subscription) Close() error {
	if err := s.reader.Close(); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to close consumer")
	}
	return nil
}

// InitStream initializes a stream
func (s *Service) InitStream(ctx context.Context, name string, subjects []string, opts StreamOpts) error {
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
func (s *Service) DeleteStream(ctx context.Context, topic string) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := client.stream.StreamInfo(topic, nats.Context(ctx)); err != nil {
		if !strings.Contains(err.Error(), "not found") {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to get stream")
		}
	} else {
		if err := client.stream.DeleteStream(topic, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to delete stream")
		}
	}
	return nil
}
