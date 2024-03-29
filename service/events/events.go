package events

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	kafkaerr "github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/ktime"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// StreamOpts are opts for streams
	StreamOpts struct {
		Partitions     int
		Replicas       int
		ReplicaQuorum  int
		RetentionAge   time.Duration
		RetentionBytes int
		MaxMsgBytes    int
	}

	// ConsumerOpts are opts for event stream consumers
	ConsumerOpts struct {
		MaxBytes         int
		RebalanceTimeout time.Duration
	}

	// Msg is a subscription message
	Msg struct {
		Topic     string
		Key       string
		Value     []byte
		Partition int
		Offset    int
		Time      time.Time
		Record    any
	}

	// PublishMsg is a message for writing
	PublishMsg struct {
		Topic string
		Key   string
		Value []byte
		Time  time.Time
	}

	// Subscription manages an active subscription
	Subscription interface {
		ReadMsg(ctx context.Context) (*Msg, error)
		MsgUnassigned(msg Msg) <-chan struct{}
		Commit(ctx context.Context, msg Msg) error
		Close(ctx context.Context) error
	}

	// Events is an events service with at least once semantics
	Events interface {
		Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error)
		Publish(ctx context.Context, msgs ...PublishMsg) error
		InitStream(ctx context.Context, topic string, opts StreamOpts) error
		DeleteStream(ctx context.Context, topic string) error
	}

	kafkaClient struct {
		client    *kgo.Client
		admclient *kadm.Client
		auth      secretAuth
	}

	Service struct {
		lc         *lifecycle.Lifecycle[kafkaClient]
		clientname string
		appname    string
		appversion string
		addr       string
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbmaxfail  int
		wg         *ksync.WaitGroup
	}

	subscription struct {
		topic    string
		group    string
		log      *klog.LevelLogger
		reader   *kgo.Client
		mu       sync.RWMutex
		assigned map[int32]chan struct{}
		closed   bool
	}
)

// New creates a new events service
func New() *Service {
	return &Service{
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "9092")
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 3)
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
	s.config = r
	s.clientname = r.Config().Instance
	s.appname = r.Config().Appname
	s.appversion = r.Config().Version.String()

	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config",
		klog.AString("addr", s.addr),
		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
	)

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))
	s.lc = lifecycle.New(
		ctx,
		s.handleGetClient,
		s.closeClient,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[kafkaClient]) {
	// Check client auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create events client"))
	}
	// Regardless of whether we were able to successfully retrieve a client, if
	// there is a client then ping the event stream. This allows vault to be
	// temporarily unavailable without disrupting the client connections.
	username := ""
	if client != nil {
		err = s.ping(ctx, client.client)
		if err == nil {
			s.hbfailed = 0
			return
		}
		username = client.auth.Username
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping event stream"),
			klog.AString("addr", s.addr),
			klog.AString("username", username),
		)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to event stream"),
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

var (
	// ErrConn is returned on a connection error
	ErrConn errConn
	// ErrClient is returned for unknown client errors
	ErrClient errClient
	// ErrClientClosed is returned when the client has been closed
	ErrClientClosed errClientClosed
	// ErrPartitionUnassigned is returned when the client has been unassigned the partition
	ErrPartitionUnassigned errPartitionUnassigned
	// ErrInvalidMsg is returned when the message is malformed
	ErrInvalidMsg errInvalidMsg
	// ErrReadEmpty is returned when no messages have been read
	ErrReadEmpty errReadEmpty
	// ErrNotFound is returned when the object is not found
	ErrNotFound errNotFound
)

type (
	errConn                struct{}
	errClient              struct{}
	errClientClosed        struct{}
	errPartitionUnassigned struct{}
	errInvalidMsg          struct{}
	errReadEmpty           struct{}
	errNotFound            struct{}
)

func (e errConn) Error() string {
	return "Events connection error"
}

func (e errClient) Error() string {
	return "Events client error"
}

func (e errClientClosed) Error() string {
	return "Events client closed"
}

func (e errPartitionUnassigned) Error() string {
	return "Partition unassigned"
}

func (e errInvalidMsg) Error() string {
	return "Invalid message"
}

func (e errReadEmpty) Error() string {
	return "No messages"
}

func (e errNotFound) Error() string {
	return "Not found"
}

type (
	secretAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.State[kafkaClient]) (*kafkaClient, error) {
	var secret secretAuth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if secret.Username == "" {
			return client, kerrors.WithKind(nil, governor.ErrInvalidConfig, "Empty auth")
		}
		if client != nil && secret == client.auth {
			return client, nil
		}
	}
	authMechanism := scram.Auth{
		User: secret.Username,
		Pass: secret.Password,
	}

	kClient, err := kgo.NewClient(append(s.commonOpts(authMechanism), []kgo.Opt{
		// producer requests

		// using default of:
		// kgo.RecordPartitioner(
		// 	kgo.UniformBytesPartitioner(1<<16, true, true, nil),
		// ),
		// partition by murmur2 hash of key if exists
		// if not exists, batch requests to a random partition and switch
		// partitions every 64KB

		kgo.ProducerBatchCompression(
			// in order of preference
			kgo.ZstdCompression(),
			kgo.Lz4Compression(),
			kgo.SnappyCompression(),
			kgo.GzipCompression(),
			kgo.NoCompression(),
		),

		// using default of:
		// kgo.MaxBufferedRecords(10000),
		// record production will block until the buffered records are flushed

		// using default of:
		// kgo.ProduceRequestTimeout(10 * time.Second),
		// in addition to RequestTimeoutOverhead, this tracks how long kafka
		// brokers have to respond to a produced record

		// using default of:
		// kgo.ProducerBatchMaxBytes(1000012), // 1MB
		// maximum message bytes, i.e. max message size for any topic and partition

		kgo.RecordDeliveryTimeout(10 * time.Second), // timeout on overall produce request on batch
		kgo.RecordRetries(32),                       // retry limit on producing records on failure
		kgo.RequiredAcks(kgo.AllISRAcks()),          // require all in-sync replicas to ack

		// using default of:
		// kgo.UnknownTopicRetries(4),
		// fail record production on receiving consecutive UNKNOWN_TOPIC_OR_PARTITION errors
	}...)...)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to create event stream client")
	}
	if err := s.ping(ctx, kClient); err != nil {
		kClient.Close()
		s.config.InvalidateSecret("auth")
		return nil, err
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to event stream",
		klog.AString("addr", s.addr),
		klog.AString("username", secret.Username),
	)

	client := &kafkaClient{
		client:    kClient,
		admclient: kadm.NewClient(kClient),
		auth:      secret,
	}
	m.Store(client)

	return client, nil
}

func (s *Service) commonOpts(auth scram.Auth) []kgo.Opt {
	netDialer := &net.Dialer{
		Timeout:   5 * time.Second,
		KeepAlive: 5 * time.Second,
	}
	return []kgo.Opt{
		kgo.ClientID(s.clientname),
		kgo.SoftwareNameAndVersion(s.appname, s.appversion),
		kgo.SeedBrokers(s.addr),
		kgo.SASL(auth.AsSha512Mechanism()),

		// connections
		kgo.Dialer(netDialer.DialContext),

		// using default of:
		// kgo.ConnIdleTimeout(20 * time.Second),
		// minimum idle time before the connection is closed

		// reading and writing data

		// using default of:
		// kgo.BrokerMaxReadBytes(100MiB),
		// maximum response size from the broker

		// using default of:
		// kgo.BrokerMaxWriteBytes(100MiB),
		// maximum request size to the broker

		kgo.RequestRetries(16),
		kgo.RequestTimeoutOverhead(10 * time.Second), // request.timeout.ms

		// do not specify RetryBackoffFn use default exponential backoff with
		// jitter, 250ms to 2.5s max

		kgo.RetryTimeout(30 * time.Second), // retry requests for this long

		// metadata
		kgo.MetadataMaxAge(1 * time.Minute),         // cache metadata for up to 1 min
		kgo.MetadataMinAge(2500 * time.Millisecond), // cache metadata for at least 2.5 seconds
	}
}

func (s *Service) ping(ctx context.Context, client *kgo.Client) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		return kerrors.WithKind(err, ErrConn, "Failed to connect to event stream")
	}
	return nil
}

func (s *Service) closeClient(ctx context.Context, client *kafkaClient) {
	if client != nil {
		client.client.Close()
		s.log.Info(ctx, "Closed event stream connection",
			klog.AString("addr", s.addr),
			klog.AString("username", client.auth.Username),
		)
	}
}

func (s *Service) getClient(ctx context.Context) (*kafkaClient, error) {
	if client := s.lc.Load(ctx); client != nil {
		return client, nil
	}

	client, err := s.lc.Construct(ctx)
	if err != nil {
		return nil, err
	}
	return client, nil
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
		return kerrors.WithKind(nil, ErrConn, "Events service not ready")
	}
	return nil
}

// NewMsgs creates new publish messages for the same topic and key
func NewMsgs(topic string, key string, values ...[]byte) []PublishMsg {
	if len(values) == 0 {
		return nil
	}

	m := make([]PublishMsg, 0, len(values))
	for _, i := range values {
		m = append(m, PublishMsg{
			Topic: topic,
			Key:   key,
			Value: i,
		})
	}
	return m
}

// Publish publishes an event
func (s *Service) Publish(ctx context.Context, msgs ...PublishMsg) error {
	if len(msgs) == 0 {
		return nil
	}

	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Round(0)
	recs := make([]*kgo.Record, 0, len(msgs))
	for _, i := range msgs {
		t := i.Time
		if t.IsZero() {
			t = now
		}
		recs = append(recs, &kgo.Record{
			Topic:     i.Topic,
			Key:       []byte(i.Key),
			Value:     i.Value,
			Timestamp: t,
		})
	}
	if err := client.client.ProduceSync(ctx, recs...).FirstErr(); err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to publish messages to event stream")
	}
	return nil
}

// Subscribe subscribes to an event stream
func (s *Service) Subscribe(ctx context.Context, topic, group string, opts ConsumerOpts) (Subscription, error) {
	client, err := s.getClient(ctx)
	if err != nil {
		return nil, err
	}
	if opts.MaxBytes == 0 {
		opts.MaxBytes = 1 << 20 // 1MB
	}
	if opts.RebalanceTimeout == 0 {
		opts.RebalanceTimeout = 30 * time.Second
	}

	sub := &subscription{
		topic: topic,
		group: group,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("subscriber",
			klog.AString("events.topic", topic),
			klog.AString("events.group", group),
		)),
		assigned: map[int32]chan struct{}{},
		closed:   false,
	}

	authMechanism := scram.Auth{
		User: client.auth.Username,
		Pass: client.auth.Password,
	}
	reader, err := kgo.NewClient(append(s.commonOpts(authMechanism), []kgo.Opt{
		// consumer topic
		kgo.ConsumeTopics(topic),

		// consumer group
		kgo.ConsumerGroup(group),

		// using default of:
		// kgo.Balancers(kgo.CooperativeStickyBalancer()),

		kgo.ConsumeResetOffset(
			kgo.NewOffset().AfterMilli(time.Now().Round(0).UnixMilli()),
		), // consume requests after now
		kgo.RebalanceTimeout(opts.RebalanceTimeout),
		kgo.OnPartitionsAssigned(sub.onAssigned),
		kgo.OnPartitionsRevoked(sub.onRevoked),
		kgo.OnPartitionsLost(sub.onLost),

		// liveness
		kgo.HeartbeatInterval(3 * time.Second),
		kgo.SessionTimeout(30 * time.Second),

		// commits
		kgo.AutoCommitInterval(2500 * time.Millisecond),
		kgo.AutoCommitMarks(), // only commit marked messages

		// consumer requests
		kgo.FetchMinBytes(1),
		kgo.FetchMaxBytes(int32(opts.MaxBytes)),
		kgo.FetchMaxPartitionBytes(int32(opts.MaxBytes)),
		kgo.FetchMaxWait(5 * time.Second),

		// transactions
		kgo.FetchIsolationLevel(kgo.ReadCommitted()), // only read committed transactions
		kgo.RequireStableFetchOffsets(),              // do not allow offsets past uncommitted transaction
	}...)...)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to create event stream client")
	}

	sub.reader = reader

	sub.log.Info(ctx, "Added subscriber")
	return sub, nil
}

func (s *subscription) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// MsgUnassigned returns a channel that closes if a message is unassigned from the consumer
func (s *subscription) MsgUnassigned(msg Msg) <-chan struct{} {
	if msg.Record == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	record, ok := msg.Record.(*kgo.Record)
	if !ok {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	if record.Topic != s.topic {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	ch, ok := s.assigned[record.Partition]
	if !ok {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return ch
}

func (s *subscription) onAssigned(ctx context.Context, client *kgo.Client, assigned map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic, partitions := range assigned {
		if topic != s.topic {
			continue
		}
		for _, i := range partitions {
			if _, ok := s.assigned[i]; !ok {
				s.assigned[i] = make(chan struct{})
			}
		}
	}
}

func (s *subscription) onRevoked(ctx context.Context, client *kgo.Client, revoked map[string][]int32) {
	s.rmPartitions(revoked)
	// must commit any marked but uncommitted messages
	if err := client.CommitMarkedOffsets(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithKind(err, ErrClient, "Failed to commit offsets on revoke"))
	}
}

func (s *subscription) onLost(ctx context.Context, client *kgo.Client, lost map[string][]int32) {
	s.rmPartitions(lost)
}

func (s *subscription) rmPartitions(partitions map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic, partitions := range partitions {
		if topic != s.topic {
			continue
		}
		for _, i := range partitions {
			if ch, ok := s.assigned[i]; ok {
				close(ch)
				delete(s.assigned, i)
			}
		}
	}
}

// ReadMsg reads a message
func (s *subscription) ReadMsg(ctx context.Context) (*Msg, error) {
	if s.isClosed() {
		return nil, kerrors.WithKind(nil, ErrClientClosed, "Client closed")
	}

	fetches := s.reader.PollRecords(ctx, 1)
	if fetches.IsClientClosed() {
		return nil, kerrors.WithKind(nil, ErrClientClosed, "Client closed")
	}
	if err := fetches.Err0(); err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to read message")
	}
	iter := fetches.RecordIter()
	if iter.Done() {
		return nil, kerrors.WithKind(nil, ErrReadEmpty, "No messages")
	}
	m := iter.Next()
	return &Msg{
		Topic:     m.Topic,
		Key:       string(m.Key),
		Value:     m.Value,
		Partition: int(m.Partition),
		Offset:    int(m.Offset),
		Time:      m.Timestamp.UTC(),
		Record:    m,
	}, nil
}

// Commit commits a new message offset
func (s *subscription) Commit(ctx context.Context, msg Msg) error {
	if msg.Record == nil {
		return kerrors.WithKind(nil, ErrInvalidMsg, "Invalid message")
	}
	record, ok := msg.Record.(*kgo.Record)
	if !ok {
		return kerrors.WithKind(nil, ErrInvalidMsg, "Invalid message")
	}
	if record.Topic != s.topic {
		return kerrors.WithKind(nil, ErrInvalidMsg, "Invalid message")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return kerrors.WithKind(nil, ErrClientClosed, "Client closed")
	}
	if _, ok := s.assigned[record.Partition]; !ok {
		return kerrors.WithKind(nil, ErrPartitionUnassigned, "Unassigned partition")
	}
	s.reader.MarkCommitRecords(record)
	return nil
}

// Close closes the subscription
func (s *subscription) Close(ctx context.Context) error {
	if s.isClosed() {
		return nil
	}

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	// unlock early to prevent any deadlock due to kafka lib implementation
	// changes with Close
	s.mu.Unlock()

	// must commit any marked but uncommitted messages
	if err := s.reader.CommitMarkedOffsets(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithKind(err, ErrClient, "Failed to commit offsets on revoke"))
	}
	s.reader.Close()
	s.log.Info(ctx, "Closed subscriber")
	return nil
}

func optInt(a int) *string {
	return kadm.StringPtr(strconv.Itoa(a))
}

func (s *Service) checkStream(ctx context.Context, client *kadm.Client, topic string) (*kadm.TopicDetail, error) {
	res, err := client.ListTopics(ctx, topic)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrClient, "Failed to get topic info")
	}
	resTopic := res[topic]
	if resTopic.Topic != topic {
		return nil, kerrors.WithKind(nil, ErrNotFound, "Topic not found")
	}
	if err := resTopic.Err; err != nil {
		if errors.Is(err, kafkaerr.UnknownTopicOrPartition) {
			return nil, kerrors.WithKind(err, ErrNotFound, "Topic not found")
		}
		return nil, kerrors.WithKind(err, ErrClient, "Failed to get topic info")
	}
	return &resTopic, nil
}

// InitStream initializes a stream
func (s *Service) InitStream(ctx context.Context, topic string, opts StreamOpts) error {
	client, err := s.getClient(ctx)
	if err != nil {
		return err
	}
	if info, err := s.checkStream(ctx, client.admclient, topic); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		res, err := client.admclient.CreateTopics(ctx, int32(opts.Partitions), int16(opts.Replicas), map[string]*string{
			"min.insync.replicas": optInt(opts.ReplicaQuorum),
			"retention.ms":        optInt(int(opts.RetentionAge.Milliseconds())),
			"retention.bytes":     optInt(opts.RetentionBytes),
			"max.message.bytes":   optInt(opts.MaxMsgBytes),
		}, topic)
		if err != nil {
			return kerrors.WithKind(err, ErrClient, "Failed to create topic")
		}
		resTopic := res[topic]
		if resTopic.Topic != topic {
			return kerrors.WithKind(nil, ErrClient, "Failed to create topic")
		}
		if resTopic.Err != nil {
			return kerrors.WithKind(resTopic.Err, ErrClient, "Failed to create topic")
		}
	} else {
		res, err := client.admclient.AlterTopicConfigs(ctx, []kadm.AlterConfig{
			{Op: kadm.SetConfig, Name: "min.insync.replicas", Value: optInt(opts.ReplicaQuorum)},
			{Op: kadm.SetConfig, Name: "retention.ms", Value: optInt(int(opts.RetentionAge.Milliseconds()))},
			{Op: kadm.SetConfig, Name: "retention.bytes", Value: optInt(opts.RetentionBytes)},
			{Op: kadm.SetConfig, Name: "max.message.bytes", Value: optInt(opts.MaxMsgBytes)},
		}, topic)
		if err != nil {
			return kerrors.WithKind(err, ErrClient, "Failed to update topic")
		}
		var resTopic *kadm.AlterConfigsResponse
		for _, i := range res {
			if i.Name == topic {
				k := i
				resTopic = &k
				break
			}
		}
		if resTopic == nil {
			return kerrors.WithKind(nil, ErrClient, "Failed to update topic")
		}
		if resTopic.Err != nil {
			return kerrors.WithKind(resTopic.Err, ErrClient, "Failed to update topic")
		}
		numPartitions := len(info.Partitions)
		if opts.Partitions > numPartitions {
			res, err := client.admclient.UpdatePartitions(ctx, opts.Partitions, topic)
			if err != nil {
				return kerrors.WithKind(err, ErrClient, "Failed to update topic partitions")
			}
			resTopic := res[topic]
			if resTopic.Topic != topic {
				return kerrors.WithKind(nil, ErrClient, "Failed to update topic partitions")
			}
			if resTopic.Err != nil {
				return kerrors.WithKind(resTopic.Err, ErrClient, "Failed to update topic partitions")
			}
		} else if opts.Partitions < numPartitions {
			s.log.Warn(ctx, "May not specify fewer partitions",
				klog.AString("topic", topic),
			)
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
	if _, err := s.checkStream(ctx, client.admclient, topic); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return err
		}
		return nil
	}
	res, err := client.admclient.DeleteTopics(ctx, topic)
	if err != nil {
		return kerrors.WithKind(err, ErrClient, "Failed to delete topic")
	}
	resTopic := res[topic]
	if resTopic.Topic != topic {
		return kerrors.WithKind(err, ErrClient, "Failed to delete topic")
	}
	if resTopic.Err != nil {
		return kerrors.WithKind(resTopic.Err, ErrClient, "Failed to delete topic")
	}
	return nil
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
		ev         Events
		log        *klog.LevelLogger
		tracer     governor.Tracer
		topic      string
		group      string
		opts       ConsumerOpts
		handler    Handler
		dlqhandler Handler
		maxdeliver int
	}
)

// Handle implements [Handler]
func (f HandlerFunc) Handle(ctx context.Context, m Msg) error {
	return f(ctx, m)
}

// NewWatcher creates a new watcher
func NewWatcher(ev Events, log klog.Logger, tracer governor.Tracer, topic, group string, opts ConsumerOpts, handler Handler, dlqhandler Handler, maxdeliver int) *Watcher {
	return &Watcher{
		ev: ev,
		log: klog.NewLevelLogger(log.Sublogger("watcher",
			klog.AString("events.topic", topic),
			klog.AString("events.group", group),
		)),
		tracer:     tracer,
		topic:      topic,
		group:      group,
		opts:       opts,
		handler:    handler,
		dlqhandler: dlqhandler,
		maxdeliver: maxdeliver,
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
		sub, err := w.ev.Subscribe(ctx, w.topic, w.group, w.opts)
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
		w.consumeMsg(ctx, sub, *m, opts)
		delay = opts.MinBackoff
	}
}

func (w *Watcher) consumeMsg(ctx context.Context, sub Subscription, m Msg, opts WatchOpts) {
	ctx = klog.CtxWithAttrs(ctx,
		klog.AInt("events.partition", m.Partition),
		klog.AInt("events.offset", m.Offset),
		klog.AInt64("events.time_us", m.Time.UnixMicro()),
		klog.AString("events.time", m.Time.UTC().Format(time.RFC3339Nano)),
		klog.AString("events.lreqid", w.tracer.LReqID()),
	)

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	wg.Add(1)
	go func() {
		defer wg.Done()
		select {
		case <-ctx.Done():
		case <-sub.MsgUnassigned(m):
			cancel(ErrPartitionUnassigned)
		}
	}()

	delay := opts.MinBackoff
	count := 0
	handledMsg := false
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		if !handledMsg {
			count++
			isDlq := w.dlqhandler != nil && count > w.maxdeliver
			var handler Handler
			if isDlq {
				handler = w.dlqhandler
			} else {
				handler = w.handler
			}

			msgctx := klog.CtxWithAttrs(ctx,
				klog.ABool("events.dlq", isDlq),
				klog.AInt("events.delivered", count),
			)
			start := time.Now()
			if err := handler.Handle(msgctx, m); err != nil {
				duration := time.Since(start)
				w.log.Err(msgctx, kerrors.WithMsg(err, "Failed executing handler"),
					klog.AInt64("duration_ms", duration.Milliseconds()),
				)
				if errors.Is(context.Cause(msgctx), ErrPartitionUnassigned) {
					return
				}
				if err := ktime.After(msgctx, delay); err != nil {
					return
				}
				delay = min(delay*2, opts.MaxBackoff)
				continue
			}
			duration := time.Since(start)
			handledMsg = true
			delay = opts.MinBackoff
			w.log.Info(msgctx, "Handled message",
				klog.AInt64("duration_ms", duration.Milliseconds()),
			)
		}
		if err := sub.Commit(ctx, m); err != nil {
			w.log.Err(ctx, kerrors.WithMsg(err, "Failed to commit message"))
			if errors.Is(err, ErrClientClosed) {
				return
			}
			if errors.Is(context.Cause(ctx), ErrPartitionUnassigned) || errors.Is(err, ErrPartitionUnassigned) || errors.Is(err, ErrInvalidMsg) {
				return
			}
			if err := ktime.After(ctx, delay); err != nil {
				return
			}
			delay = min(delay*2, opts.MaxBackoff)
			continue
		}
		w.log.Info(ctx, "Committed message")
		return
	}
}
