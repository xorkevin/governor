package events

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/twmb/franz-go/pkg/kadm"
	"github.com/twmb/franz-go/pkg/kerr"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/governor/util/uid"
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
		record    *kgo.Record
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
		IsAssigned(msg Msg) bool
		Commit(ctx context.Context, msg Msg) error
		Close(ctx context.Context) error
		IsPermanentlyClosed() bool
	}

	// Events is a service wrapper around an event stream client
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
		instance   string
		addr       string
		config     governor.SecretReader
		log        *klog.LevelLogger
		hbfailed   int
		hbmaxfail  int
		wg         *ksync.WaitGroup
	}

	subscription struct {
		s        *Service
		topic    string
		group    string
		log      *klog.LevelLogger
		reader   *kgo.Client
		mu       *sync.RWMutex
		assigned map[int32]struct{}
		closed   bool
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
		hbfailed: 0,
		wg:       ksync.NewWaitGroup(),
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
	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"events.addr":       s.addr,
		"events.hbinterval": hbinterval.String(),
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
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[kafkaClient]) {
	// Check client auth expiry, and reinit client if about to be expired
	client, err := m.Construct(ctx)
	if err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to create events client"), nil)
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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to ping event stream"), klog.Fields{
			"events.addr":     s.addr,
			"events.username": username,
		})
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max pings to event stream"), klog.Fields{
		"events.addr":     s.addr,
		"events.username": username,
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
	// ErrorClientClosed is returned when the client has been closed
	ErrorClientClosed struct{}
	// ErrorPartitionUnassigned is returned when the client has been unassigned the partition
	ErrorPartitionUnassigned struct{}
	// ErrorInvalidMsg is returned when the message is malformed
	ErrorInvalidMsg struct{}
	// ErrorReadEmpty is returned when no messages have been read
	ErrorReadEmpty struct{}
	// ErrorNotFound is returned when the object is not found
	ErrorNotFound struct{}
)

func (e ErrorConn) Error() string {
	return "Events connection error"
}

func (e ErrorClient) Error() string {
	return "Events client error"
}

func (e ErrorClientClosed) Error() string {
	return "Events client closed"
}

func (e ErrorPartitionUnassigned) Error() string {
	return "Partition unassigned"
}

func (e ErrorInvalidMsg) Error() string {
	return "Invalid message"
}

func (e ErrorReadEmpty) Error() string {
	return "No messages"
}

func (e ErrorNotFound) Error() string {
	return "Not found"
}

type (
	secretAuth struct {
		Username string `mapstructure:"username"`
		Password string `mapstructure:"password"`
	}
)

func (s *Service) handleGetClient(ctx context.Context, m *lifecycle.Manager[kafkaClient]) (*kafkaClient, error) {
	var secret secretAuth
	{
		client := m.Load(ctx)
		if err := s.config.GetSecret(ctx, "auth", 0, &secret); err != nil {
			return client, kerrors.WithMsg(err, "Invalid secret")
		}
		if secret.Username == "" {
			return client, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "Empty auth")
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
		kgo.RecordPartitioner(
			// partition by murmur2 hash of key if exists
			// if not exists, batch requests to a random partition and switch
			// partitions every 65KB
			kgo.UniformBytesPartitioner(1<<16, true, true, nil),
		),
		kgo.ProducerBatchCompression(
			// in order of preference
			kgo.ZstdCompression(),
			kgo.Lz4Compression(),
			kgo.SnappyCompression(),
			kgo.GzipCompression(),
			kgo.NoCompression(),
		),
		kgo.MaxBufferedRecords(8192),
		kgo.ProduceRequestTimeout(10 * time.Second), // in addition to RequestTimeoutOverhead
		kgo.ProducerBatchMaxBytes(1 << 20),          // 1MB
		kgo.RecordDeliveryTimeout(30 * time.Second),
		kgo.RecordRetries(16),
		kgo.RequiredAcks(kgo.AllISRAcks()), // require all in-sync replicas to ack
		kgo.UnknownTopicRetries(3),
	}...)...)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create event stream client")
	}
	if err := s.ping(ctx, kClient); err != nil {
		kClient.Close()
		s.config.InvalidateSecret("auth")
		return nil, err
	}

	m.Stop(ctx)

	s.log.Info(ctx, "Established connection to event stream", klog.Fields{
		"events.addr":     s.addr,
		"events.username": secret.Username,
	})

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
		kgo.ClientID(s.clientname + "-" + s.instance),
		kgo.SeedBrokers(s.addr),
		kgo.SASL(auth.AsSha512Mechanism()),
		// connections
		kgo.Dialer(netDialer.DialContext),
		kgo.ConnIdleTimeout(30 * time.Second),
		// reading and writing data
		kgo.BrokerMaxReadBytes(1 << 25),  // 32MB
		kgo.BrokerMaxWriteBytes(1 << 25), // 32MB
		// need to set otherwise error with max fetch greater than broker max read
		kgo.FetchMaxBytes(int32(1 << 20)), // 1MB
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
		return kerrors.WithKind(err, ErrorConn{}, "Failed to connect to event stream")
	}
	return nil
}

func (s *Service) closeClient(ctx context.Context, client *kafkaClient) {
	if client != nil {
		client.client.Close()
		s.log.Info(ctx, "Closed event stream connection", klog.Fields{
			"events.addr":     s.addr,
			"events.username": client.auth.Username,
		})
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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, ErrorConn{}, "Events service not ready")
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
		return kerrors.WithKind(err, ErrorClient{}, "Failed to publish messages to event stream")
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
		s:     s,
		topic: topic,
		group: group,
		log: klog.NewLevelLogger(s.log.Logger.Sublogger("subscriber", klog.Fields{
			"events.topic": topic,
			"events.group": group,
		})),
		mu:       &sync.RWMutex{},
		assigned: map[int32]struct{}{},
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
		kgo.Balancers(kgo.CooperativeStickyBalancer()),
		kgo.ConsumeResetOffset(
			kgo.NewOffset().AfterMilli(time.Now().Round(0).UnixMilli()),
		), // consume requests after now
		kgo.RebalanceTimeout(opts.RebalanceTimeout),
		kgo.OnPartitionsAssigned(sub.onAssigned),
		kgo.OnPartitionsRevoked(sub.onRevoked),
		kgo.OnPartitionsLost(sub.onLost),
		// liveness
		kgo.HeartbeatInterval(3 * time.Second),
		kgo.SessionTimeout(15 * time.Second),
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
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to create event stream client")
	}

	sub.reader = reader

	sub.log.Info(ctx, "Added subscriber", nil)
	return sub, nil
}

func (s *subscription) isClosed() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.closed
}

// IsAssigned returns if a message is assigned to the consumer
func (s *subscription) IsAssigned(msg Msg) bool {
	if msg.record == nil || msg.record.Topic != s.topic {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false
	}
	_, ok := s.assigned[msg.record.Partition]
	return ok
}

func (s *subscription) onAssigned(ctx context.Context, client *kgo.Client, assigned map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic, partitions := range assigned {
		if topic != s.topic {
			continue
		}
		for _, i := range partitions {
			s.assigned[i] = struct{}{}
		}
	}
}

func (s *subscription) onRevoked(ctx context.Context, client *kgo.Client, revoked map[string][]int32) {
	func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for topic, partitions := range revoked {
			if topic != s.topic {
				continue
			}
			for _, i := range partitions {
				delete(s.assigned, i)
			}
		}
	}()
	// must commit any marked but uncommitted messages
	if err := client.CommitUncommittedOffsets(ctx); err != nil {
		s.log.Err(ctx, kerrors.WithKind(err, ErrorClient{}, "Failed to commit offsets on revoke"), nil)
	}
}

func (s *subscription) onLost(ctx context.Context, client *kgo.Client, lost map[string][]int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for topic, partitions := range lost {
		if topic != s.topic {
			continue
		}
		for _, i := range partitions {
			delete(s.assigned, i)
		}
	}
}

// ReadMsg reads a message
func (s *subscription) ReadMsg(ctx context.Context) (*Msg, error) {
	if s.isClosed() {
		return nil, kerrors.WithKind(nil, ErrorClientClosed{}, "Client closed")
	}

	fetches := s.reader.PollRecords(ctx, 1)
	if err := fetches.Err0(); err != nil {
		err = kerrors.WithKind(err, ErrorClient{}, "Failed to read message")
		if !kerr.IsRetriable(err) {
			return nil, kerrors.WithKind(err, ErrorClientClosed{}, "Client closed")
		}
		return nil, err
	}
	iter := fetches.RecordIter()
	if iter.Done() {
		return nil, kerrors.WithKind(nil, ErrorReadEmpty{}, "No messages")
	}
	m := iter.Next()
	return &Msg{
		Topic:     m.Topic,
		Key:       string(m.Key),
		Value:     m.Value,
		Partition: int(m.Partition),
		Offset:    int(m.Offset),
		Time:      m.Timestamp.UTC(),
		record:    m,
	}, nil
}

// Commit commits a new message offset
func (s *subscription) Commit(ctx context.Context, msg Msg) error {
	if s.isClosed() {
		return kerrors.WithKind(nil, ErrorClientClosed{}, "Client closed")
	}
	if !s.IsAssigned(msg) {
		return kerrors.WithKind(nil, ErrorPartitionUnassigned{}, "Unassigned partition")
	}
	if msg.record == nil {
		return kerrors.WithKind(nil, ErrorInvalidMsg{}, "Invalid message")
	}
	s.reader.MarkCommitRecords(msg.record)
	return nil
}

// Close closes the subscription
func (s *subscription) Close(ctx context.Context) error {
	if s.isClosed() {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.reader.Close()
	s.closed = true
	s.log.Info(ctx, "Closed subscriber", nil)
	return nil
}

// IsPermanentlyClosed returns if the client is closed
func (s *subscription) IsPermanentlyClosed() bool {
	return s.isClosed()
}

func optInt(a int) *string {
	return kadm.StringPtr(strconv.Itoa(a))
}

func (s *Service) checkStream(ctx context.Context, client *kadm.Client, topic string) (*kadm.TopicDetail, error) {
	res, err := client.ListTopics(ctx, topic)
	if err != nil {
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to get topic info")
	}
	resTopic := res[topic]
	if resTopic.Topic != topic {
		return nil, kerrors.WithKind(nil, ErrorNotFound{}, "Topic not found")
	}
	if err := resTopic.Err; err != nil {
		if errors.Is(err, kerr.UnknownTopicOrPartition) {
			return nil, kerrors.WithKind(err, ErrorNotFound{}, "Topic not found")
		}
		return nil, kerrors.WithKind(err, ErrorClient{}, "Failed to get topic info")
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
		if !errors.Is(err, ErrorNotFound{}) {
			return err
		}
		res, err := client.admclient.CreateTopics(ctx, int32(opts.Partitions), int16(opts.Replicas), map[string]*string{
			"min.insync.replicas": optInt(opts.ReplicaQuorum),
			"retention.ms":        optInt(int(opts.RetentionAge.Milliseconds())),
			"retention.bytes":     optInt(opts.RetentionBytes),
			"max.message.bytes":   optInt(opts.MaxMsgBytes),
		}, topic)
		if err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to create topic")
		}
		resTopic := res[topic]
		if resTopic.Topic != topic {
			return kerrors.WithKind(nil, ErrorClient{}, "Failed to create topic")
		}
		if resTopic.Err != nil {
			return kerrors.WithKind(resTopic.Err, ErrorClient{}, "Failed to create topic")
		}
	} else {
		res, err := client.admclient.AlterTopicConfigs(ctx, []kadm.AlterConfig{
			{Op: kadm.SetConfig, Name: "min.insync.replicas", Value: optInt(opts.ReplicaQuorum)},
			{Op: kadm.SetConfig, Name: "retention.ms", Value: optInt(int(opts.RetentionAge.Milliseconds()))},
			{Op: kadm.SetConfig, Name: "retention.bytes", Value: optInt(opts.RetentionBytes)},
			{Op: kadm.SetConfig, Name: "max.message.bytes", Value: optInt(opts.MaxMsgBytes)},
		}, topic)
		if err != nil {
			return kerrors.WithKind(err, ErrorClient{}, "Failed to update topic")
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
			return kerrors.WithKind(nil, ErrorClient{}, "Failed to update topic")
		}
		if resTopic.Err != nil {
			return kerrors.WithKind(resTopic.Err, ErrorClient{}, "Failed to update topic")
		}
		numPartitions := len(info.Partitions)
		if opts.Partitions > numPartitions {
			res, err := client.admclient.UpdatePartitions(ctx, opts.Partitions, topic)
			if err != nil {
				return kerrors.WithKind(err, ErrorClient{}, "Failed to update topic partitions")
			}
			resTopic := res[topic]
			if resTopic.Topic != topic {
				return kerrors.WithKind(nil, ErrorClient{}, "Failed to update topic partitions")
			}
			if resTopic.Err != nil {
				return kerrors.WithKind(resTopic.Err, ErrorClient{}, "Failed to update topic partitions")
			}
		} else if opts.Partitions < numPartitions {
			s.log.Warn(ctx, "May not specify fewer partitions", klog.Fields{
				"events.topic": topic,
			})
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
		if !errors.Is(err, ErrorNotFound{}) {
			return err
		}
		return nil
	}
	res, err := client.admclient.DeleteTopics(ctx, topic)
	if err != nil {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to delete topic")
	}
	resTopic := res[topic]
	if resTopic.Topic != topic {
		return kerrors.WithKind(err, ErrorClient{}, "Failed to delete topic")
	}
	if resTopic.Err != nil {
		return kerrors.WithKind(resTopic.Err, ErrorClient{}, "Failed to delete topic")
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
		MaxBackoff time.Duration
	}

	// Watcher watches over a subscription
	Watcher struct {
		ev          Events
		log         *klog.LevelLogger
		topic       string
		group       string
		opts        ConsumerOpts
		handler     Handler
		dlqhandler  Handler
		maxdeliver  int
		reqidprefix string
		reqcount    *atomic.Uint32
	}
)

// Handle implements [Handler]
func (f HandlerFunc) Handle(ctx context.Context, m Msg) error {
	return f(ctx, m)
}

// NewWatcher creates a new watcher
func NewWatcher(ev Events, log klog.Logger, topic, group string, opts ConsumerOpts, handler Handler, dlqhandler Handler, maxdeliver int, reqidprefix string) *Watcher {
	return &Watcher{
		ev: ev,
		log: klog.NewLevelLogger(log.Sublogger("watcher", klog.Fields{
			"events.topic": topic,
			"events.group": group,
		})),
		topic:       topic,
		group:       group,
		opts:        opts,
		handler:     handler,
		dlqhandler:  dlqhandler,
		maxdeliver:  maxdeliver,
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
			sub, err := w.ev.Subscribe(ctx, w.topic, w.group, w.opts)
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
					if errors.Is(err, ErrorClientClosed{}) {
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

				count := 1
				for {
					msgctx := klog.WithFields(ctx, klog.Fields{
						"events.topic":     m.Topic,
						"events.partition": m.Partition,
						"events.offset":    m.Offset,
						"events.time":      m.Time.UTC().Format(time.RFC3339Nano),
						"events.delivered": count,
						"events.lreqid":    w.lreqID(),
					})
					if w.dlqhandler != nil && count > w.maxdeliver {
						count++
						start := time.Now()
						if err := w.dlqhandler.Handle(msgctx, *m); err != nil {
							duration := time.Since(start)
							w.log.Err(msgctx, kerrors.WithMsg(err, "Failed executing dlq handler"), klog.Fields{
								"events.duration_ms": duration.Milliseconds(),
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
						w.log.Info(msgctx, "DLQ handled message", klog.Fields{
							"events.duration_ms": duration.Milliseconds(),
						})
						if err := sub.Commit(msgctx, *m); err != nil {
							w.log.Err(msgctx, kerrors.WithMsg(err, "Failed to commit message"), nil)
							if errors.Is(err, ErrorClientClosed{}) {
								return
							}
							if errors.Is(err, ErrorPartitionUnassigned{}) || errors.Is(err, ErrorInvalidMsg{}) {
								break
							}
							select {
							case <-msgctx.Done():
								return
							case <-time.After(delay):
								delay = min(delay*2, opts.MaxBackoff)
								continue
							}
						}
						w.log.Info(msgctx, "Committed message", nil)
					} else {
						count++
						start := time.Now()
						if err := w.handler.Handle(msgctx, *m); err != nil {
							duration := time.Since(start)
							w.log.Err(msgctx, kerrors.WithMsg(err, "Failed executing subscription handler"), klog.Fields{
								"events.duration_ms": duration.Milliseconds(),
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
							"events.duration_ms": duration.Milliseconds(),
						})
						if err := sub.Commit(msgctx, *m); err != nil {
							w.log.Err(msgctx, kerrors.WithMsg(err, "Failed to commit message"), nil)
							if errors.Is(err, ErrorClientClosed{}) {
								return
							}
							if errors.Is(err, ErrorPartitionUnassigned{}) || errors.Is(err, ErrorInvalidMsg{}) {
								break
							}
							select {
							case <-msgctx.Done():
								return
							case <-time.After(delay):
								delay = min(delay*2, opts.MaxBackoff)
								continue
							}
						}
						w.log.Info(msgctx, "Committed message", nil)
						break
					}
				}
				delay = watchStartDelay
			}
		}()
	}
}
