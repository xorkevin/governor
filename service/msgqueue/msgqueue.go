package msgqueue

import (
	"context"
	"fmt"
	"github.com/labstack/echo"
	"github.com/nats-io/stan.go"
	"net/http"
	"sync/atomic"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/uid"
)

const (
	uidSize = 16
)

type (
	// Msgqueue is a service wrapper around a nats streaming queue client instance
	Msgqueue interface {
		governor.Service
		SubscribeQueue(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error)
		Publish(queueid string, msgdata []byte) error
	}

	service struct {
		clientid string
		queue    stan.Conn
		logger   governor.Logger
		done     <-chan struct{}
	}

	Subscription interface {
		Close() error
	}

	subscription struct {
		logger    governor.Logger
		sub       stan.Subscription
		lastAcked uint64
		worker    func(data []byte)
	}
)

// New creates a new msgqueue service
func New() (Msgqueue, error) {
	clientid, err := uid.New(uidSize)
	if err != nil {
		return nil, governor.NewError("Failed to create new uid", http.StatusInternalServerError, err)
	}
	clientidstr := clientid.Base64()

	return &service{
		clientid: clientidstr,
	}, nil
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("cluster", "nss")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	conf := r.GetStrMap("")

	conn, err := stan.Connect(conf["cluster"], s.clientid, func(options *stan.Options) error {
		options.NatsURL = "nats://" + conf["host"] + ":" + conf["port"]
		return nil
	})
	if err != nil {
		l.Error("Failed connect nats", map[string]string{
			"err": err.Error(),
		})
		return err
	}
	s.queue = conn

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		if err := s.queue.Close(); err != nil {
			s.logger.Error("msgqueue: failed to close msgqueue connection", map[string]string{
				"error": err.Error(),
			})
		} else {
			s.logger.Info("msgqueue: closed connection", nil)
		}
		done <- struct{}{}
	}()
	s.done = done

	l.Info(fmt.Sprintf("msgqueue: establish connection to %s:%s", conf["host"], conf["port"]), nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.logger.Warn("msgqueue: failed to stop", nil)
	}
}

func (s *service) Health() error {
	return nil
}

// SubscribeQueue subscribes to a queue
func (s *service) SubscribeQueue(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error) {
	msub := &subscription{
		logger: s.logger,
		worker: worker,
	}
	sub, err := s.queue.QueueSubscribe(queueid, queuegroup, msub.subscriber, stan.DurableName(queuegroup+"-durable"), stan.SetManualAckMode())
	if err != nil {
		return nil, governor.NewError("Failed to create subscription", http.StatusInternalServerError, err)
	}
	msub.sub = sub
	return msub, nil
}

func (s *service) Publish(queueid string, msgdata []byte) error {
	if err := s.queue.Publish(queueid, msgdata); err != nil {
		return governor.NewError("Failed to publish message: ", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *subscription) subscriber(msg *stan.Msg) {
	for {
		local := s.lastAcked
		if msg.Sequence <= local {
			return
		}
		if atomic.CompareAndSwapUint64(&s.lastAcked, local, msg.Sequence) {
			s.worker(msg.Data)
			if err := msg.Ack(); err != nil {
				s.logger.Error("msgqueue: subscriber: Fail ack message", nil)
			}
			return
		}
	}
}

// Close closes the subscription
func (s *subscription) Close() error {
	if err := s.sub.Close(); err != nil {
		return governor.NewError("Failed to close subscription", http.StatusInternalServerError, err)
	}
	return nil
}
