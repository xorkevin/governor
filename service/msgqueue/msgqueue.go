package msgqueue

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/nats-io/go-nats-streaming"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
	"sync/atomic"
)

type (
	// Msgqueue is a service wrapper around a nats streaming queue client instance
	Msgqueue interface {
		governor.Service
		SubscribeQueue(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, *governor.Error)
		Publish(queueid string, msgdata []byte) *governor.Error
		Close() *governor.Error
	}

	msgQueue struct {
		logger *logrus.Logger
		queue  stan.Conn
	}

	Subscription interface {
		Close() *governor.Error
	}

	subscription struct {
		logger    *logrus.Logger
		sub       stan.Subscription
		lastAcked uint64
		worker    func(data []byte)
	}
)

const (
	moduleID = "nats"
)

// New creates a new cache service
func New(c governor.Config, l *logrus.Logger) (Msgqueue, error) {
	v := c.Conf()
	rconf := v.GetStringMapString("nats")

	clientid, err := uid.NewU(8, 8)
	if err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}
	clientidstr := strings.TrimRight(clientid.Base64(), "=")

	var conn stan.Conn
	if connection, err := stan.Connect(rconf["cluster"], clientidstr, func(options *stan.Options) error {
		options.NatsURL = "nats://" + rconf["host"] + ":" + rconf["port"]
		return nil
	}); err == nil {
		conn = connection
	} else {
		l.Errorf("error creating connection to NATS: %s\n", err)
		return nil, err
	}

	l.Infof("msgqueue: connected to %s:%s", rconf["host"], rconf["port"])
	l.Info("initialized msgqueue")

	return &msgQueue{
		logger: l,
		queue:  conn,
	}, nil
}

// Mount is a place to mount routes to satisfy the Service interface
func (q *msgQueue) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted msgqueue")
	return nil
}

// Health is a health check for the service
func (q *msgQueue) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (q *msgQueue) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	return nil
}

const (
	moduleIDsubscriber = moduleID + "Subscription.subscriber"
)

func (s *subscription) subscriber(msg *stan.Msg) {
	for {
		local := s.lastAcked
		if msg.Sequence <= local {
			return
		}
		if atomic.CompareAndSwapUint64(&s.lastAcked, local, msg.Sequence) {
			if err := msg.Ack(); err != nil {
				s.logger.Error(governor.NewError(moduleIDsubscriber, "Failed to ack message: "+err.Error(), 0, http.StatusInternalServerError))
			}
			s.worker(msg.Data)
			return
		}
	}
}

const (
	moduleIDSubcriptionClose = moduleID + ".Subscription.Close"
)

// Close closes the subscription
func (s *subscription) Close() *governor.Error {
	if err := s.sub.Close(); err != nil {
		return governor.NewError(moduleIDSubcriptionClose, "Failed to close subscription: "+err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDSubscribeQueue = moduleID + ".SubscribeQueue"
)

// SubscribeQueue subscribes to a queue
func (q *msgQueue) SubscribeQueue(queueid, queuegroup string, worker func(msgdata []byte)) (Subscription, *governor.Error) {
	msub := &subscription{
		logger: q.logger,
		worker: worker,
	}
	sub, err := q.queue.QueueSubscribe(queueid, queuegroup, msub.subscriber, stan.DurableName(queuegroup+"-durable"), stan.SetManualAckMode())
	if err != nil {
		return nil, governor.NewError(moduleIDSubscribeQueue, "Failed to create subscription: "+err.Error(), 0, http.StatusInternalServerError)
	}
	msub.sub = sub
	return msub, nil
}

const (
	moduleIDPublish = moduleID + ".Publish"
)

func (q *msgQueue) Publish(queueid string, msgdata []byte) *governor.Error {
	if err := q.queue.Publish(queueid, msgdata); err != nil {
		return governor.NewError(moduleIDPublish, "Failed to publish message: "+err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

const (
	moduleIDClose = moduleID + ".Close"
)

// Close closes the client connection
func (q *msgQueue) Close() *governor.Error {
	if err := q.queue.Close(); err != nil {
		return governor.NewError(moduleIDClose, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}