package pubsub

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
	"net/http"
	"time"
	"xorkevin.dev/governor"
)

type (
	WorkerFunc func(msgdata []byte) error

	// Pubsub is a service wrapper around a nats pub sub client
	Pubsub interface {
		Subscribe(channel string, worker WorkerFunc) (Subscription, error)
		SubscribeGroup(channel, group string, worker WorkerFunc) (Subscription, error)
		Publish(channel string, msgdata []byte) error
	}

	Service interface {
		governor.Service
		Pubsub
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

	Subscription interface {
		Close() error
	}

	subscription struct {
		logger governor.Logger
		sub    *nats.Subscription
		worker WorkerFunc
	}
)

// New creates a new pubsub service
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
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.config = r

	conf := r.GetStrMap("")
	s.addr = fmt.Sprintf("%s:%s", conf["host"], conf["port"])
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
			return
		}
		s.ready = false
		s.config.InvalidateSecret("auth")
	}
	if _, err := s.handleGetClient(); err != nil {
		s.logger.Error("failed to create pubsub client", map[string]string{
			"error":      err.Error(),
			"actiontype": "createpubsubclient",
		})
	}
}

func (s *service) handleGetClient() (*nats.Conn, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth := authsecret["password"].(string)
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	canary := make(chan struct{})
	conn, err := nats.Connect(fmt.Sprintf("nats://%s@%s", auth, s.addr),
		nats.NoReconnect(),
		nats.PingInterval(time.Duration(s.hbinterval)*time.Second),
		nats.MaxPingsOutstanding(s.hbmaxfail),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			s.logger.Error("Lost connection to pubsub", map[string]string{
				"error":      err.Error(),
				"actiontype": "pingpubsub",
			})
			close(canary)
		}))
	if err != nil {
		return nil, governor.NewError("Failed to connect to pubsub", http.StatusInternalServerError, err)
	}

	s.client = conn
	s.auth = auth
	s.ready = true
	s.canary = canary
	s.logger.Info(fmt.Sprintf("established connection to %s", s.addr), nil)
	return s.client, nil
}

func (s *service) closeClient() {
	if s.client == nil {
		return
	}
	s.client.Close()
	s.logger.Info("closed pubsub connection", map[string]string{
		"actiontype": "closepubsubok",
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
		return nil, governor.NewError("Pubsub service shutdown", http.StatusInternalServerError, nil)
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
		return governor.NewError("Pubsub service not ready", http.StatusInternalServerError, nil)
	}
	return nil
}

// Subscribe subscribes to a channel
func (s *service) Subscribe(channel string, worker WorkerFunc) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent": "subscriber",
	})
	msub := &subscription{
		logger: l,
		worker: worker,
	}
	sub, err := s.client.Subscribe(channel, msub.subscriber)
	if err != nil {
		return nil, governor.NewError("Failed to create subscription to channel", http.StatusInternalServerError, err)
	}
	msub.sub = sub
	return msub, nil
}

// SubscribeGroup subscribes to a queue group
func (s *service) SubscribeGroup(channel, group string, worker WorkerFunc) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent": "subscriber",
	})
	msub := &subscription{
		logger: l,
		worker: worker,
	}
	sub, err := s.client.QueueSubscribe(channel, group, msub.subscriber)
	if err != nil {
		return nil, governor.NewError("Failed to create subscription to queue group", http.StatusInternalServerError, err)
	}
	msub.sub = sub
	return msub, nil
}

func (s *service) Publish(channel string, msgdata []byte) error {
	if err := s.client.Publish(channel, msgdata); err != nil {
		return governor.NewError("Failed to publish message: ", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *subscription) subscriber(msg *nats.Msg) {
	s.worker(msg.Data)
}

// Close closes the subscription
func (s *subscription) Close() error {
	if err := s.sub.Unsubscribe(); err != nil {
		return governor.NewError("Failed to close subscription", http.StatusInternalServerError, err)
	}
	return nil
}
