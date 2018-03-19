package websocket

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/hackform/governor"
	"github.com/sirupsen/logrus"
	"net/http"
)

const (
	moduleID = "websocket"
)

type (
	// Websocket is a websocket management service
	Websocket interface {
		Subscribe(conn *Conn)
		Unsubscribe(conn *Conn)
		Broadcast(channel string, msg interface{}) *governor.Error
		NewConn(channels []string, w http.ResponseWriter, r *http.Request) (*Conn, *governor.Error)
	}

	websocketService struct {
		channels map[string]*channel
		upgrader *websocket.Upgrader
		logger   *logrus.Logger
	}

	channel struct {
		id        string
		listeners map[*Conn]bool
	}

	// Conn describes a websocket connection subscribed to multiple channels
	Conn struct {
		w        Websocket
		conn     *websocket.Conn
		channels []string
	}
)

// New creates a new Websocket service
func New(conf governor.Config, l *logrus.Logger) Websocket {
	l.Info("initialized websocket service")

	return &websocketService{
		channels: make(map[string]*channel),
		upgrader: &websocket.Upgrader{},
	}
}

func (w *websocketService) getChannel(channel string) *channel {
	if c, ok := w.channels[channel]; ok {
		return c
	}
	c := newChannel(channel)
	w.channels[channel] = c
	return c
}

func (w *websocketService) removeChannel(channel string) {
	delete(w.channels, channel)
}

// Subscribe adds a websocket connection to a channel
func (w *websocketService) Subscribe(conn *Conn) {
	for _, channel := range conn.channels {
		c := w.getChannel(channel)
		c.listeners[conn] = true
	}
}

// Unsubscribe removes a websocket connection from a channel
func (w *websocketService) Unsubscribe(conn *Conn) {
	for _, channel := range conn.channels {
		if c, ok := w.channels[channel]; ok {
			delete(c.listeners, conn)
		}
	}
}

const (
	moduleIDBroadcast = moduleID + ".Broadcast"
)

// Broadcast sends a message to all websocket connections in a channel
func (w *websocketService) Broadcast(channel string, msg interface{}) *governor.Error {
	c, validChannel := w.channels[channel]
	if !validChannel {
		return governor.NewError(moduleIDBroadcast, "Invalid channel", 2, http.StatusNotFound)
	}
	m, err := json.Marshal(msg)
	if err != nil {
		return governor.NewError(moduleIDBroadcast, err.Error(), 0, http.StatusInternalServerError)
	}
	p, err := websocket.NewPreparedMessage(websocket.TextMessage, m)
	if err != nil {
		return governor.NewError(moduleIDBroadcast, err.Error(), 0, http.StatusInternalServerError)
	}
	for listener := range c.listeners {
		if err := listener.conn.WritePreparedMessage(p); err != nil {
			w.logger.WithFields(logrus.Fields{
				"origin": moduleIDBroadcast,
			}).Errorf("Failed to send websocket message: %s", err.Error())
			listener.Close()
		}
	}
	return nil
}

func newChannel(id string) *channel {
	return &channel{
		id:        id,
		listeners: make(map[*Conn]bool),
	}
}

const (
	moduleIDConn = moduleID + ".Conn"

	moduleIDConnNew = moduleIDConn + ".New"
)

// NewConn establishes a new websocket connection from an existing http connection
func (w *websocketService) NewConn(channels []string, rw http.ResponseWriter, r *http.Request) (*Conn, *governor.Error) {
	ws, err := w.upgrader.Upgrade(rw, r, nil)
	if err != nil {
		return nil, governor.NewErrorUser(moduleIDConnNew, err.Error(), 0, http.StatusBadRequest)
	}

	c := &Conn{
		w:        w,
		conn:     ws,
		channels: channels,
	}
	w.Subscribe(c)
	return c, nil
}

const (
	moduleIDConnRead = moduleIDConn + ".Read"
)

// Read reads a message to json
func (c *Conn) Read(msg interface{}) *governor.Error {
	if err := c.conn.ReadJSON(msg); err != nil {
		c.Close()
		return governor.NewError(moduleIDConnRead, err.Error(), 0, http.StatusInternalServerError)
	}
	return nil
}

// Close closes the websocket connection
func (c *Conn) Close() {
	c.w.Unsubscribe(c)
	if c.conn.Close() != nil {
	}
}
