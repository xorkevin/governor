package msgqueue

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/stan.go"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
)

const (
	uidSize = 16
)

type (
	// Msgqueue is a service wrapper around a nats streaming queue client instance
	Msgqueue interface {
		Subscribe(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error)
		Publish(queueid string, msgdata []byte) error
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

	service struct {
		client     stan.Conn
		auth       string
		clientid   string
		clusterid  string
		addr       string
		hbinterval int
		hbmaxfail  int
		config     governor.SecretReader
		logger     governor.Logger
		ops        chan getOp
		done       <-chan struct{}
	}

	Subscription interface {
		Close() error
	}

	subscription struct {
		s      *service
		logger governor.Logger
		sub    stan.Subscription
		worker func(data []byte)
	}
)

// New creates a new msgqueue service
func New() Service {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("auth", "")
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("cluster", "nss")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 5)
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
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

	conf := r.GetStrMap("")
	s.clusterid = conf["cluster"]
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
	for {
		select {
		case <-ctx.Done():
			s.closeClient()
			return
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

func (s *service) handleGetClient() (stan.Conn, error) {
	authsecret, err := s.config.GetSecret("auth")
	if err != nil {
		return nil, err
	}
	auth := authsecret["password"].(string)
	if auth == s.auth {
		return s.client, nil
	}

	s.closeClient()

	conn, err := stan.Connect(s.clusterid, s.clientid,
		stan.NatsURL(fmt.Sprintf("nats://%s@%s", auth, s.addr)),
		stan.Pings(s.hbinterval, s.hbmaxfail),
		stan.SetConnectionLostHandler(func(_ stan.Conn, err error) {
			s.logger.Error("Lost connection to msgqueue", map[string]string{
				"error": err.Error(),
			})
			s.config.InvalidateSecret("auth")
		}))
	if err != nil {
		return nil, governor.NewError("Failed to connect to msgqueue", http.StatusInternalServerError, err)
	}

	s.client = conn
	s.auth = auth
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
	return nil
}

// SubscribeQueue subscribes to a queue
func (s *service) Subscribe(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent":      "subscriber",
		"queueid":    queueid,
		"queuegroup": queuegroup,
	})
	msub := &subscription{
		s:      s,
		logger: l,
		worker: worker,
	}
	client, err := s.getClient()
	if err != nil {
		return nil, err
	}
	sub, err := client.QueueSubscribe(queueid, queuegroup, msub.subscriber, stan.DurableName(queuegroup+"-durable"), stan.SetManualAckMode())
	if err != nil {
		return nil, governor.NewError("Failed to create subscription to queue", http.StatusInternalServerError, err)
	}
	msub.sub = sub
	return msub, nil
}

func (s *service) Publish(queueid string, msgdata []byte) error {
	client, err := s.getClient()
	if err != nil {
		return err
	}
	if err := client.Publish(queueid, msgdata); err != nil {
		return governor.NewError("Failed to publish message: ", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *subscription) subscriber(msg *stan.Msg) {
	s.worker(msg.Data)
	if err := msg.Ack(); err != nil {
		s.logger.Error("Failed to ack message", map[string]string{
			"error":      err.Error(),
			"actiontype": "ackmessage",
		})
	}
}

// Close closes the subscription
func (s *subscription) Close() error {
	if err := s.sub.Close(); err != nil {
		return governor.NewError("Failed to close subscription", http.StatusInternalServerError, err)
	}
	return nil
}
