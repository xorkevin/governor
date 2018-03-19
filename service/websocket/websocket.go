package websocket

import (
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
		Subscribe(channel string, ws string) *governor.Error
		Unsubscribe(channel string, ws string) *governor.Error
		Broadcast(channel string, msg string) *governor.Error
	}

	websocketService struct {
		channels map[string]*channel
		upgrader *websocket.Upgrader
	}

	channel struct {
		listeners map[*websocket.Conn]bool
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

func (w *websocketService) Subscribe(channel string, ws string) *governor.Error {
	_, validChannel := w.channels[channel]
	if !validChannel {
		return governor.NewErrorUser(moduleID, "Invalid channel", 0, http.StatusNotFound)
	}
	return nil
}

func (w *websocketService) Unsubscribe(channel string, ws string) *governor.Error {
	_, validChannel := w.channels[channel]
	if !validChannel {
		return governor.NewErrorUser(moduleID, "Invalid channel", 0, http.StatusNotFound)
	}
	return nil
}

func (w *websocketService) Broadcast(channel string, msg string) *governor.Error {
	_, validChannel := w.channels[channel]
	if !validChannel {
		return governor.NewErrorUser(moduleID, "Invalid channel", 0, http.StatusNotFound)
	}
	return nil
}
