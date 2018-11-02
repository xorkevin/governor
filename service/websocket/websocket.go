package websocket

import (
	"encoding/json"
	"github.com/gorilla/websocket"
	"github.com/hackform/governor"
	"net/http"
	"sync"
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
		channels sync.Map
		upgrader *websocket.Upgrader
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
func New(conf governor.Config, l governor.Logger) Websocket {
	l.Info("initialized websocket service", moduleID, "initialize websocket service", 0, nil)

	return &websocketService{
		channels: sync.Map{},
		upgrader: &websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (w *websocketService) upsertChannel(s string) *channel {
	c := newChannel(s)
	a, _ := w.channels.LoadOrStore(s, c)
	return a.(*channel)
}

func (w *websocketService) getChannel(s string) (*channel, bool) {
	a, b := w.channels.Load(s)
	if !b {
		return nil, false
	}
	return a.(*channel), true
}

func (w *websocketService) removeChannel(s string) {
	w.channels.Delete(s)
}

// Subscribe adds a websocket connection to a channel
func (w *websocketService) Subscribe(conn *Conn) {
	for _, channel := range conn.channels {
		c := w.upsertChannel(channel)
		c.listeners[conn] = true
	}
}

// Unsubscribe removes a websocket connection from a channel
func (w *websocketService) Unsubscribe(conn *Conn) {
	for _, channel := range conn.channels {
		if c, ok := w.getChannel(channel); ok {
			delete(c.listeners, conn)
			if len(c.listeners) == 0 {
				// TODO: listeners may not be 0 here
				w.removeChannel(channel)
			}
		}
	}
}

const (
	moduleIDBroadcast = moduleID + ".Broadcast"
)

// Broadcast sends a message to all websocket connections in a channel
func (w *websocketService) Broadcast(channel string, msg interface{}) *governor.Error {
	c, validChannel := w.getChannel(channel)
	if !validChannel {
		return nil
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
