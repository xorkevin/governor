package msgqueue

import (
	"context"
	"fmt"
	"github.com/nats-io/stan.go"
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
)

const (
	uidSize = 16
)

type (
	WorkerFunc func(msgdata []byte) error

	// Msgqueue is a service wrapper around a nats streaming client instance
	Msgqueue interface {
		Subscribe(channel, group string, ackwait time.Duration, inflight int, worker WorkerFunc) (Subscription, error)
		Publish(channel string, msgdata []byte) error
	}

	Service interface {
		governor.Service
		Msgqueue
	}

	getClientRes struct {
		client stan.Conn
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
		client     stan.Conn
		auth       string
		clientid   string
		clusterid  string
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

	Subscription interface {
		Unsubscribe() error
		Close() error
	}

	subscription struct {
		s        *service
		channel  string
		group    string
		ackwait  time.Duration
		inflight int
		worker   WorkerFunc
		logger   governor.Logger
		sub      stan.Subscription
	}

	ctxKeyMsgqueue struct{}
)

// GetCtxMsgqueue returns a Msgqueue from the context
func GetCtxMsgqueue(ctx context.Context) (Msgqueue, error) {
	v := ctx.Value(ctxKeyMsgqueue{})
	if v == nil {
		return nil, governor.NewError("Msgqueue not found in context", http.StatusInternalServerError, nil)
	}
	return v.(Msgqueue), nil
}

// SetCtxMsgqueue sets a Msgqueue in the context
func SetCtxMsgqueue(ctx context.Context, q Msgqueue) context.Context {
	return context.WithValue(ctx, ctxKeyMsgqueue{}, q)
}

// New creates a new msgqueue service
func New() Service {
	return &service{
		ops:    make(chan getOp),
		subops: make(chan subOp),
		subs:   map[*subscription]struct{}{},
		ready:  false,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("cluster", "nats-streaming")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	clientid, err := uid.New(uidSize)
	if err != nil {
		return governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	s.clientid = clientid.Base64()

	s.clusterid = r.GetStr("cluster")
	s.addr = fmt.Sprintf("%s:%s", r.GetStr("host"), r.GetStr("port"))
	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")

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
		s.logger.Error("failed to create msgqueue client", map[string]string{
			"error":      err.Error(),
			"actiontype": "createmsgqueueclient",
		})
	}
}

func (s *service) updateSubs() {
	for k := range s.subs {
		if k.ok() {
			continue
		}
		if err := k.init(s.client); err != nil {
			s.logger.Error("failed to subscribe to channel", map[string]string{
				"error":      err.Error(),
				"actiontype": "createmsgqueuesuberr",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("subscribed to channel", map[string]string{
				"actiontype": "createmsgqueuesubok",
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
			s.logger.Error("failed to close subscription", map[string]string{
				"error":      err.Error(),
				"actiontype": "closemsgqueuesuberr",
				"channel":    k.channel,
				"group":      k.group,
			})
		} else {
			s.logger.Info("closed subscription", map[string]string{
				"actiontype": "closemsgqueuesubok",
				"channel":    k.channel,
				"group":      k.group,
			})
		}
	}
}

func (s *service) handleGetClient() (stan.Conn, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth, ok := authsecret["password"].(string)
	if !ok {
		return nil, governor.NewError("Invalid secret", http.StatusInternalServerError, nil)
	}
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	canary := make(chan struct{})
	conn, err := stan.Connect(s.clusterid, s.clientid,
		stan.NatsURL(fmt.Sprintf("nats://%s@%s", auth, s.addr)),
		stan.Pings(s.hbinterval, s.hbmaxfail),
		stan.SetConnectionLostHandler(func(_ stan.Conn, err error) {
			s.logger.Error("Lost connection to msgqueue", map[string]string{
				"error":      err.Error(),
				"actiontype": "pingmsgqueue",
			})
			close(canary)
		}))
	if err != nil {
		return nil, governor.NewError("Failed to connect to msgqueue", http.StatusInternalServerError, err)
	}

	s.client = conn
	s.auth = auth
	s.ready = true
	s.canary = canary
	s.logger.Info(fmt.Sprintf("established connection to %s %s with client %s", s.addr, s.clusterid, s.clientid), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	if err := s.client.Close(); err != nil {
		s.logger.Error("failed to close msgqueue connection", map[string]string{
			"error":      err.Error(),
			"actiontype": "closemsgqueueerr",
			"address":    s.addr,
			"clusterid":  s.clusterid,
			"clientid":   s.clientid,
		})
	} else {
		s.logger.Info("closed msgqueue connection", map[string]string{
			"actiontype": "closemsgqueueok",
			"address":    s.addr,
			"clusterid":  s.clusterid,
			"clientid":   s.clientid,
		})
	}
}

func (s *service) getClient() (stan.Conn, error) {
	res := make(chan getClientRes)
	op := getOp{
		res: res,
	}
	select {
	case <-s.done:
		return nil, governor.NewError("Msgqueue service shutdown", http.StatusInternalServerError, nil)
	case s.ops <- op:
		v := <-res
		return v.client, v.err
	}
}

func (s *service) Setup(req governor.ReqSetup) error {
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
		l.Warn("failed to stop", nil)
	}
}

func (s *service) Health() error {
	if !s.ready {
		return governor.NewError("Msgqueue service not ready", http.StatusInternalServerError, nil)
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
func (s *service) Subscribe(channel, group string, ackwait time.Duration, inflight int, worker WorkerFunc) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": channel,
		"group":   group,
	})
	sub := &subscription{
		s:        s,
		channel:  channel,
		group:    group,
		ackwait:  ackwait,
		inflight: inflight,
		worker:   worker,
		logger:   l,
	}
	s.addSub(sub)
	return sub, nil
}

// Publish publishes to a channel
func (s *service) Publish(channel string, msgdata []byte) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Publish(channel, msgdata); err != nil {
		return governor.NewError("Failed to publish message", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *subscription) init(client stan.Conn) error {
	sub, err := client.QueueSubscribe(s.channel, s.group,
		s.subscriber,
		stan.DurableName(s.group+"-durable"),
		stan.SetManualAckMode(),
		stan.AckWait(s.ackwait),
		stan.MaxInflight(s.inflight))
	if err != nil {
		return err
	}
	s.sub = sub
	return nil
}

func (s *subscription) deinit() error {
	k := s.sub
	s.sub = nil
	if err := k.Close(); err != nil {
		return err
	}
	return nil
}

func (s *subscription) ok() bool {
	return s.sub != nil
}

func (s *subscription) subscriber(msg *stan.Msg) {
	if err := s.worker(msg.Data); err != nil {
		s.logger.Error("Failed to execute message handler", map[string]string{
			"error":      err.Error(),
			"actiontype": "execworker",
		})
		return
	}
	if err := msg.Ack(); err != nil {
		s.logger.Error("Failed to ack message", map[string]string{
			"error":      err.Error(),
			"actiontype": "ackmessage",
		})
	}
}

// Unsubscribe removes the subscription
func (s *subscription) Unsubscribe() error {
	s.s.rmSub(s)
	if s.sub == nil {
		return nil
	}
	if err := s.sub.Unsubscribe(); err != nil {
		return governor.NewError("Failed to unsubscribe", http.StatusInternalServerError, err)
	}
	return nil
}

// Close closes the subscription
func (s *subscription) Close() error {
	s.s.rmSub(s)
	if s.sub == nil {
		return nil
	}
	if err := s.sub.Close(); err != nil {
		return governor.NewError("Failed to close subscription", http.StatusInternalServerError, err)
	}
	return nil
}
