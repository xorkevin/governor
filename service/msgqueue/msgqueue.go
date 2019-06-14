package msgqueue

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/nats-io/go-nats-streaming"
	"net/http"
	"sync/atomic"
)

type (
	// Msgqueue is a service wrapper around a nats streaming queue client instance
	Msgqueue interface {
		governor.Service
		SubscribeQueue(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error)
		Publish(queueid string, msgdata []byte) error
		Close() error
	}

	msgQueue struct {
		logger governor.Logger
		queue  stan.Conn
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

const (
	moduleID = "nats"
)

// New creates a new msgqueue service
func New(c governor.Config, l governor.Logger) (Msgqueue, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("nats")

	clientid, err := uid.NewU(8, 8)
	if err != nil {
		return nil, governor.NewError("Failed to get new uid", http.StatusInternalServerError, err)
	}
	clientidstr := clientid.Base64()

	var conn stan.Conn
	if connection, err := stan.Connect(rconf["cluster"], clientidstr, func(options *stan.Options) error {
		options.NatsURL = "nats://" + rconf["host"] + ":" + rconf["port"]
		return nil
	}); err == nil {
		conn = connection
	} else {
		l.Error("Fail connect nats", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	l.Info(fmt.Sprintf("msgqueue: establish connection to %s:%s", rconf["host"], rconf["port"]), nil)
	l.Info("initialize msgqueue serivce", nil)

	return &msgQueue{
		logger: l,
		queue:  conn,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (q *msgQueue) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mount msgqueue service", nil)
	return nil
}

// Health is a health check for the service
func (q *msgQueue) Health() error {
	return nil
}

// Setup is run on service setup
func (q *msgQueue) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	return nil
}

func (s *subscription) subscriber(msg *stan.Msg) {
	for {
		local := s.lastAcked
		if msg.Sequence <= local {
			return
		}
		if atomic.CompareAndSwapUint64(&s.lastAcked, local, msg.Sequence) {
			if err := msg.Ack(); err != nil {
				s.logger.Error("msgqueue: subscriber: Fail ack message", nil)
			}
			s.worker(msg.Data)
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

// SubscribeQueue subscribes to a queue
func (q *msgQueue) SubscribeQueue(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, error) {
	msub := &subscription{
		logger: q.logger,
		worker: worker,
	}
	sub, err := q.queue.QueueSubscribe(queueid, queuegroup, msub.subscriber, stan.DurableName(queuegroup+"-durable"), stan.SetManualAckMode())
	if err != nil {
		return nil, governor.NewError("Failed to create subscription", http.StatusInternalServerError, err)
	}
	msub.sub = sub
	return msub, nil
}

func (q *msgQueue) Publish(queueid string, msgdata []byte) error {
	if err := q.queue.Publish(queueid, msgdata); err != nil {
		return governor.NewError("Failed to publish message: ", http.StatusInternalServerError, err)
	}
	return nil
}

const (
	moduleIDClose = moduleID + ".Close"
)

// Close closes the client connection
func (q *msgQueue) Close() error {
	if err := q.queue.Close(); err != nil {
		return governor.NewError("Failed to close client connection", http.StatusInternalServerError, err)
	}
	return nil
}
