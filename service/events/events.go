package events

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl/scram"
	"xorkevin.dev/governor"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// // StreamOpts are opts for streams
	// StreamOpts struct {
	// 	Replicas   int
	// 	MaxAge     time.Duration
	// 	MaxBytes   int64
	// 	MaxMsgSize int32
	// 	MaxMsgs    int64
	// }

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
		//InitStream(ctx context.Context, topic string, opts StreamOpts) error
		//DeleteStream(ctx context.Context, topic string) error
	}

	kafkaClient struct {
		dialer *kafka.Dialer
		client *kafka.Client
		writer *kafka.Writer
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
	r.SetDefault("port", "9092")
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
	var err error
	// Check client auth expiry, and reinit client if about to be expired
	if _, err = s.handleGetClient(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create events client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the event stream. This allows vault to be
	// temporarily unavailable without disrupting the client connections.
	if s.client != nil {
		err = s.ping(ctx, s.client.client)
		if err == nil {
			s.ready.Store(true)
			s.hbfailed = 0
			return
		}
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping event stream"), klog.Fields{
			"events.addr":     s.addr,
			"events.username": s.auth.Username,
		})
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to event stream"), klog.Fields{
		"events.addr":     s.addr,
		"events.username": s.auth.Username,
	})
	s.aclient.Store(nil)
	s.ready.Store(false)
	s.hbfailed = 0
	s.auth = secretAuth{}
	s.config.InvalidateSecret("auth")
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

type (
	secretAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context) (*kafkaClient, error) {
	var secret secretAuth
	if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid secret")
	}
	if secret.Username == "" {
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
	}
	if secret == s.auth {
		return s.client, nil
	}
	mechanism, err := scram.Mechanism(scram.SHA512, "username", "password")
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create scram mechanism")
	}

	s.closeClient(klog.ExtendCtx(context.Background(), ctx, nil))

	netDialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}
	dialer := &kafka.Dialer{
		ClientID:      s.clientname + "-" + s.instance,
		DialFunc:      netDialer.DialContext,
		SASLMechanism: mechanism,
	}
	transport := &kafka.Transport{
		ClientID:    s.clientname + "-" + s.instance,
		Dial:        netDialer.DialContext,
		DialTimeout: 10 * time.Second,
		IdleTimeout: 30 * time.Second,
		MetadataTTL: 6 * time.Second,
		SASL:        mechanism,
	}
	client := &kafka.Client{
		Addr:      kafka.TCP(s.addr),
		Transport: transport,
	}
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(s.addr),
		Balancer:               &kafka.Hash{},
		MaxAttempts:            16,
		BatchSize:              128,
		BatchBytes:             1 << 20,
		BatchTimeout:           125 * time.Millisecond,
		ReadTimeout:            5 * time.Second,
		WriteTimeout:           5 * time.Second,
		RequiredAcks:           kafka.RequireAll,
		Async:                  false,
		AllowAutoTopicCreation: false,
		Transport:              transport,
	}

	if err := s.ping(ctx, client); err != nil {
		return nil, err
	}

	s.client = &kafkaClient{
		dialer: dialer,
		client: client,
		writer: writer,
	}
	s.aclient.Store(s.client)
	s.auth = secret
	s.ready.Store(true)
	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"events.addr":     s.addr,
		"events.username": s.auth.Username,
	})
	return s.client, nil
}

func (s *Service) ping(ctx context.Context, client *kafka.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if _, err := client.ApiVersions(ctx, &kafka.ApiVersionsRequest{}); err != nil {
		return kerrors.WithKind(err, ErrorConn{}, "Failed to connect to event stream")
	}
	return nil
}

func (s *Service) closeClient(ctx context.Context) {
	s.aclient.Store(nil)
	if s.client != nil {
		if err := s.client.writer.Close(); err != nil {
			s.log.Err(ctx, kerrors.WithKind(err, ErrorClient{}, "Failed to close event stream connection"), klog.Fields{
				"events.addr":     s.addr,
				"events.username": s.auth.Username,
			})
		} else {
			s.log.Info(ctx, "Closed event stream connection", klog.Fields{
				"events.addr":     s.addr,
				"events.username": s.auth.Username,
			})
		}
	}
	s.client = nil
	s.auth = secretAuth{}
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
	if err := client.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
		Time:  time.Now().Round(0),
	}); err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish message to event stream")
	}
	return nil
}

// Subscribe subscribes to an event stream
func (s *Service) Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	readerconfig := kafka.ReaderConfig{
		Brokers: []string{s.addr},
		GroupID: group,
		Topic:   topic,
		Dialer:  client.dialer,
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

// // InitStream initializes a stream
// func (s *Service) InitStream(ctx context.Context, name string, subjects []string, opts StreamOpts) error {
// 	client, err := s.getClient(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	cfg := &nats.StreamConfig{
// 		Name:       name,
// 		Subjects:   subjects,
// 		Retention:  nats.LimitsPolicy,
// 		Discard:    nats.DiscardOld,
// 		Storage:    nats.FileStorage,
// 		Replicas:   opts.Replicas,
// 		MaxAge:     opts.MaxAge,
// 		MaxBytes:   opts.MaxBytes,
// 		MaxMsgSize: opts.MaxMsgSize,
// 		MaxMsgs:    opts.MaxMsgs,
// 	}
// 	if _, err := client.stream.StreamInfo(name, nats.Context(ctx)); err != nil {
// 		if !strings.Contains(err.Error(), "not found") {
// 			return kerrors.WithKind(err, ErrorClient{}, "Failed to get stream")
// 		}
// 		if _, err := client.stream.AddStream(cfg, nats.Context(ctx)); err != nil {
// 			return kerrors.WithKind(err, ErrorClient{}, "Failed to create stream")
// 		}
// 	} else {
// 		if _, err := client.stream.UpdateStream(cfg, nats.Context(ctx)); err != nil {
// 			return kerrors.WithKind(err, ErrorClient{}, "Failed to update stream")
// 		}
// 	}
// 	return nil
// }
//
// // DeleteStream deletes a stream
// func (s *Service) DeleteStream(ctx context.Context, topic string) error {
// 	client, err := s.getClient(ctx)
// 	if err != nil {
// 		return err
// 	}
// 	if _, err := client.stream.StreamInfo(topic, nats.Context(ctx)); err != nil {
// 		if !strings.Contains(err.Error(), "not found") {
// 			return kerrors.WithKind(err, ErrorClient{}, "Failed to get stream")
// 		}
// 	} else {
// 		if err := client.stream.DeleteStream(topic, nats.Context(ctx)); err != nil {
// 			return kerrors.WithKind(err, ErrorClient{}, "Failed to delete stream")
// 		}
// 	}
// 	return nil
// }
