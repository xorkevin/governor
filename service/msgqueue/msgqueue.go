package msgqueue

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/nats-io/go-nats-streaming"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

type (
	// Msgqueue is a service wrapper around a nats streaming queue client instance
	Msgqueue interface {
		governor.Service
		Queue() stan.Conn
		Close() *governor.Error
	}

	msgQueue struct {
		queue stan.Conn
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
		queue: conn,
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

// Queue returns the queue client instance
func (q *msgQueue) Queue() stan.Conn {
	return q.queue
}

// Close closes the client connection
func (q *msgQueue) Close() *governor.Error {
	if err := q.queue.Close(); err != nil {
		return governor.NewError(moduleID, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}
