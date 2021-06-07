package events

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"
	"xorkevin.dev/governor"
)

type (
	// WorkerFunc is a type alias for a subscriber handler
	WorkerFunc = func(msgdata []byte)

	// Events is a service wrapper around an event stream client
	Events interface {
		Subscribe(channel string, worker WorkerFunc) (Subscription, error)
		SubscribeGroup(channel, group string, worker WorkerFunc) (Subscription, error)
		Publish(channel string, msgdata []byte) error
	}

	// Service is an Events and governor.Service
	Service interface {
		governor.Service
		Events
	}

	getClientRes struct {
		client *nats.Conn
		err    error
	}

	getOp struct {
		res chan<- getClientRes
	}

	subOp struct {
		rm  bool
		sub *subscription
	}

	service struct {
		client     *nats.Conn
		clientname string
		auth       string
		addr       string
		config     governor.SecretReader
		logger     governor.Logger
		ops        chan getOp
		subops     chan subOp
		subs       map[*subscription]struct{}
		ready      bool
		canary     <-chan struct{}
		hbinterval int
		hbmaxfail  int
		done       <-chan struct{}
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
	return &service{
		ops:    make(chan getOp),
		subops: make(chan subOp),
		subs:   map[*subscription]struct{}{},
		ready:  false,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxEvents(inj, s)

	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 3)
}

type (
	// ErrConn is returned on a connection error
	ErrConn struct{}
	// ErrClient is returned for unknown client errors
	ErrClient struct{}
)

func (e ErrConn) Error() string {
	return "Events connection error"
}

func (e ErrClient) Error() string {
	return "Events client error"
}

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

	l.Info("loaded config", map[string]string{
		"addr":       s.addr,
		"hbinterval": strconv.Itoa(s.hbinterval),
		"hbmaxfail":  strconv.Itoa(s.hbmaxfail),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	if _, err := s.getClient(); err != nil {
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
				delete(s.subs, op.sub)
			} else {
				s.subs[op.sub] = struct{}{}
			}
		case op := <-s.ops:
			client, err := s.handleGetClient()
			op.res <- getClientRes{
				client: client,
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
			s.updateSubs()
			return
		}
		s.ready = false
		s.config.InvalidateSecret("auth")
		s.deinitSubs()
	}
	if _, err := s.handleGetClient(); err != nil {
		s.logger.Error("Failed to create events client", map[string]string{
			"error":      err.Error(),
			"actiontype": "createeventsclient",
		})
	}
}

func (s *service) updateSubs() {
	for k := range s.subs {
		if k.ok() {
			continue
		}
		if err := k.init(s.client); err != nil {
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
}

func (s *service) deinitSubs() {
	for k := range s.subs {
		if !k.ok() {
			continue
		}
		if err := k.deinit(); err != nil {
			s.logger.Error("Failed to close subscription", map[string]string{
				"error":      err.Error(),
				"actiontype": "closeeventssuberr",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("Closed subscription", map[string]string{
				"actiontype": "closeeventssubok",
				"channel":    k.channel,
				"group":      k.group,
			})
		}
	}
}

func (s *service) handleGetClient() (*nats.Conn, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth, ok := authsecret["password"].(string)
	if !ok {
		return nil, governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "Invalid secret")
	}
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	canary := make(chan struct{})
	conn, err := nats.Connect(fmt.Sprintf("nats://%s", s.addr),
		nats.Name(s.clientname),
		nats.Token(auth),
		nats.NoReconnect(),
		nats.PingInterval(time.Duration(s.hbinterval)*time.Second),
		nats.MaxPingsOutstanding(s.hbmaxfail),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			s.logger.Error("Lost connection to events", map[string]string{
				"error":      err.Error(),
				"actiontype": "pingevents",
			})
			close(canary)
		}))
	if err != nil {
		return nil, governor.ErrWithKind(err, ErrConn{}, "Failed to connect to events")
	}

	s.client = conn
	s.auth = auth
	s.ready = true
	s.canary = canary
	s.logger.Info(fmt.Sprintf("Established connection to %s", s.addr), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	s.client.Close()
	s.logger.Info("Closed events connection", map[string]string{
		"actiontype": "closeeventsok",
		"address":    s.addr,
	})
}

func (s *service) getClient() (*nats.Conn, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, governor.ErrWithKind(nil, ErrConn{}, "Events service shutdown")
	case s.ops <- op:
		v := <-res
		return v.client, v.err
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

func (s *service) addSub(sub *subscription) {
	op := subOp{
		rm:  false,
		sub: sub,
	}
	select {
	case <-s.done:
	case s.subops <- op:
	}
}

func (s *service) rmSub(sub *subscription) {
	op := subOp{
		rm:  true,
		sub: sub,
	}
	select {
	case <-s.done:
	case s.subops <- op:
	}
}

// Subscribe subscribes to a channel
func (s *service) Subscribe(channel string, worker WorkerFunc) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": channel,
	})
	sub := &subscription{
		s:       s,
		channel: channel,
		worker:  worker,
		logger:  l,
	}
	s.addSub(sub)
	return sub, nil
}

// SubscribeGroup subscribes to a queue group
func (s *service) SubscribeGroup(channel, group string, worker WorkerFunc) (Subscription, error) {
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
	s.addSub(sub)
	return sub, nil
}

func (s *service) Publish(channel string, msgdata []byte) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Publish(channel, msgdata); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to publish message")
	}
	return nil
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

func (s *subscription) deinit() error {
	k := s.sub
	s.sub = nil
	if err := k.Unsubscribe(); err != nil {
		return err
	}
	return nil
}

func (s *subscription) ok() bool {
	return s.sub != nil
}

func (s *subscription) subscriber(msg *nats.Msg) {
	s.worker(msg.Data)
}

// Close closes the subscription
func (s *subscription) Close() error {
	s.s.rmSub(s)
	if s.sub == nil {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return governor.ErrWithKind(err, ErrClient{}, "Failed to close subscription")
	}
	return nil
}
