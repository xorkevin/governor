package events

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	natsClient struct {
		client    *nats.Conn
		jetstream nats.JetStreamContext
		auth      natsauth
	}

	NatsService struct {
		lc         *lifecycle.Lifecycle[natsClient]
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

	natsSubscription struct {
		topic  string
		group  string
		log    *klog.LevelLogger
		client *nats.Conn
		sub    *nats.Subscription
	}
)

func NewNats() *NatsService {
	return &NatsService{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *NatsService) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxEvents(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
}

func (s *NatsService) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
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
		"events.addr":       s.addr,
		"events.hbinterval": s.hbinterval.String(),
		"events.hbmaxfail":  s.hbmaxfail,
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

func (s *NatsService) handlePing(ctx context.Context, m *lifecycle.Manager[natsClient]) {
	// Check client auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create events client"), nil)
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the events server. This allows vault to be
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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping events server"), klog.Fields{
			"events.addr": s.addr,
		})
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to events server"), klog.Fields{
		"events.addr": s.addr,
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
	natsauth struct {
		Password string `mapstructure:"password"`
	}
)

func (s *NatsService) handleGetClient(ctx context.Context, m *lifecycle.Manager[natsClient]) (*natsClient, error) {
	var secret natsauth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if secret.Password == "" {
			return client, kerrors.WithKind(nil, governor.ErrorInvalidConfig, "Empty auth")
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
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to connect to events")
	}
	if _, err := conn.RTT(); err != nil {
		conn.Close()
		s.config.InvalidateSecret("auth")
		return nil, kerrors.WithKind(err, ErrorConn, "Failed to connect to events")
	}
	jetstream, err := conn.JetStream()
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to connect to events stream")
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"events.addr": s.addr,
	})

	client := &natsClient{
		client:    conn,
		jetstream: jetstream,
		auth:      secret,
	}
	m.Store(client)

	return client, nil
}

func (s *NatsService) closeClient(ctx context.Context, client *natsClient) {
	if client != nil && !client.client.IsClosed() {
		client.client.Close()
		s.log.Info(ctx, "Closed events connection", klog.Fields{
			"events.addr": s.addr,
		})
	}
}

func (s *NatsService) getClient(ctx context.Context) (*nats.Conn, nats.JetStreamContext, error) {
	if client := s.lc.Load(ctx); client != nil {
		return client.client, client.jetstream, nil
	}

	client, err := s.lc.Construct(ctx)
	if err != nil {
		return nil, nil, err
	}
	return client.client, client.jetstream, nil
}

func (s *NatsService) Start(ctx context.Context) error {
	return nil
}

func (s *NatsService) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *NatsService) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *NatsService) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrorConn, "Events service not ready")
	}
	return nil
}

const (
	natsMsgKeyHeader = "events.key"
)

var (
	natsStreamNameReplacer = strings.NewReplacer(".", "_")
)

// Publish publishes to a subject
func (s *NatsService) Publish(ctx context.Context, msgs ...PublishMsg) error {
	if len(msgs) == 0 {
		return nil
	}

	_, jetstream, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	for _, i := range msgs {
		header := nats.Header{}
		header.Set(natsMsgKeyHeader, i.Key)
		if _, err := jetstream.PublishMsg(&nats.Msg{
			Subject: i.Topic,
			Header:  header,
			Data:    i.Value,
		}, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient, "Failed to publish message to event stream")
		}
	}
	return nil
}

// Subscribe subscribes to an event stream
func (s *NatsService) Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error) {
	client, jetstream, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = 1 << 20 // 1MB
	}
	if opts.RebalanceTimeout == 0 {
		opts.RebalanceTimeout = 30 * time.Second
	}

	streamName := natsStreamNameReplacer.Replace(topic)
	consumerName := natsStreamNameReplacer.Replace(group)
	now := time.Now().Round(0)
	if _, err := jetstream.AddConsumer(streamName, &nats.ConsumerConfig{
		Durable:       consumerName,
		OptStartTime:  &now,
		AckPolicy:     nats.AckExplicitPolicy,
		AckWait:       time.Nanosecond, // redeliver immediately to prevent out of order deliveries
		MaxAckPending: 1,               // only one ack pending to prevent out of order deliveries
		MaxWaiting:    1,               // only one pull fetch request in flight
	}); err != nil {
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to create consumer")
	}

	nsub, err := jetstream.PullSubscribe(
		topic,
		consumerName,
		nats.Bind(streamName, consumerName),
		nats.ManualAck(),
	)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to create subscription")
	}

	sub := &natsSubscription{
		topic: topic,
		group: group,
		log: klog.NewLevelLogger(klog.Sub(s.log.Logger, "subscriber", klog.Fields{
			"events.topic": topic,
			"events.group": group,
		})),
		client: client,
		sub:    nsub,
	}
	sub.log.Info(ctx, "Added subscription", nil)
	return sub, nil
}

func (s *natsSubscription) isClosed() bool {
	return s.sub.IsValid()
}

// IsAssigned returns if a message is assigned to the consumer
func (s *natsSubscription) IsAssigned(msg Msg) bool {
	if msg.natsmsg == nil || msg.natsmsg.Subject != s.topic {
		return false
	}
	if s.isClosed() {
		return false
	}
	return true
}

// ReadMsg reads a message
func (s *natsSubscription) ReadMsg(ctx context.Context) (*Msg, error) {
	msgs, err := s.sub.Fetch(1, ctx)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to get message")
	}
	if len(msgs) != 1 {
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to get message")
	}
	m := msgs[0]
	meta, err := m.Metadata()
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient, "Failed to get message metadata")
	}
	return &Msg{
		Topic:     m.Subject,
		Key:       m.Header.Get(natsMsgKeyHeader),
		Value:     m.Data,
		Partition: 0,
		Offset:    int(meta.Sequence.Stream),
		Time:      meta.Timestamp.UTC(),
		natsmsg:   m,
	}, nil
}

// Commit commits a new message offset
func (s *natsSubscription) Commit(ctx context.Context, msg Msg) error {
	if s.isClosed() {
		return kerrors.WithKind(nil, ErrorClientClosed, "Client closed")
	}
	if !s.IsAssigned(msg) {
		return kerrors.WithKind(nil, ErrorPartitionUnassigned, "Unassigned partition")
	}
	if msg.natsmsg == nil {
		return kerrors.WithKind(nil, ErrorInvalidMsg, "Invalid message")
	}
	if err := msg.natsmsg.Ack(); err != nil {
		s.log.Err(ctx, kerrors.WithKind(nil, ErrorClient, "Failed to ack message"), nil)
	}
	return nil
}

// Close closese the subscription
func (s *natsSubscription) Close(ctx context.Context) error {
	if s.isClosed() {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return kerrors.WithKind(err, ErrorClient, "Failed to close subscription")
	}
	s.log.Info(ctx, "Closed subscription", nil)
	return nil
}

// IsPermanentlyClosed returns if the client is closed
func (s *natsSubscription) IsPermanentlyClosed() bool {
	return s.isClosed()
}

// InitStream initializes a stream
func (s *NatsService) InitStream(ctx context.Context, topic string, opts StreamOpts) error {
	_, jetstream, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	streamName := natsStreamNameReplacer.Replace(topic)
	cfg := &nats.StreamConfig{
		Name:       streamName,
		Subjects:   []string{topic},
		Retention:  nats.LimitsPolicy,
		Discard:    nats.DiscardOld,
		Storage:    nats.FileStorage,
		Replicas:   opts.Replicas,
		MaxAge:     opts.RetentionAge,
		MaxBytes:   int64(opts.RetentionBytes),
		MaxMsgSize: int32(opts.MaxMsgBytes),
	}
	if _, err := jetstream.StreamInfo(streamName, nats.Context(ctx)); err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			return kerrors.WithKind(err, ErrorClient, "Failed to get topic info")
		}
		if _, err := jetstream.AddStream(cfg, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient, "Failed to create topic")
		}
	} else {
		if _, err := jetstream.UpdateStream(cfg, nats.Context(ctx)); err != nil {
			return kerrors.WithKind(err, ErrorClient, "Failed to update topic")
		}
	}
	return nil
}

// DeleteStream deletes a stream
func (s *NatsService) DeleteStream(ctx context.Context, topic string) error {
	_, jetstream, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if _, err := jetstream.StreamInfo(topic, nats.Context(ctx)); err != nil {
		if !errors.Is(err, nats.ErrStreamNotFound) {
			return kerrors.WithKind(err, ErrorClient, "Failed to get topic info")
		}
		return nil
	}
	if err := jetstream.DeleteStream(topic, nats.Context(ctx)); err != nil {
		return kerrors.WithKind(err, ErrorClient, "Failed to delete topic")
	}
	return nil
}
