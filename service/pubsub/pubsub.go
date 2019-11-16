package pubsub

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"github.com/nats-io/nats.go"
	"net/http"
	"xorkevin.dev/governor"
)

type (
	// Pubsub is a service wrapper around a nats pub sub client
	Pubsub interface {
		Subscribe(queueid string, worker func(msgdata []byte)) (Subscription, error)
		SubscribeGroup(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error)
		Publish(queueid string, msgdata []byte) error
	}

	Service interface {
		governor.Service
		Pubsub
	}

	service struct {
		queue  *nats.Conn
		logger governor.Logger
		done   <-chan struct{}
	}

	Subscription interface {
		Close() error
	}

	subscription struct {
		logger governor.Logger
		sub    *nats.Subscription
		worker func(data []byte)
	}
)

// New creates a new pubsub service
func New() Service {
	return &service{}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("host", "localhost")
	r.SetDefault("port", "4222")
	r.SetDefault("token", "admin")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	conn, err := nats.Connect(fmt.Sprintf("nats://%s@%s:%s", r.GetStr("token"), r.GetStr("host"), r.GetStr("port")))
	if err != nil {
		return governor.NewError("Failed to connect to nats", http.StatusInternalServerError, err)
	}
	s.queue = conn

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		l := s.logger.WithData(map[string]string{
			"phase": "stop",
		})
		s.queue.Close()
		l.Info("closed connection", nil)
		close(done)
	}()
	s.done = done

	l.Info(fmt.Sprintf("establish connection to %s:%s", r.GetStr("host"), r.GetStr("port")), nil)
	return nil
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

// Subscribe subscribes to a channel
func (s *service) Subscribe(queueid string, worker func(msgdata []byte)) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent": "subscriber",
	})
	msub := &subscription{
		logger: l,
		worker: worker,
	}
	sub, err := s.queue.Subscribe(queueid, msub.subscriber)
	if err != nil {
		return nil, governor.NewError("Failed to create subscription to channel", http.StatusInternalServerError, err)
	}
	msub.sub = sub
	return msub, nil
}

// SubscribeGroup subscribes to a queue group
func (s *service) SubscribeGroup(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error) {
	l := s.logger.WithData(map[string]string{
		"agent": "subscriber",
	})
	msub := &subscription{
		logger: l,
		worker: worker,
	}
	sub, err := s.queue.QueueSubscribe(queueid, queuegroup, msub.subscriber)
	if err != nil {
		return nil, governor.NewError("Failed to create subscription to queue group", http.StatusInternalServerError, err)
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
