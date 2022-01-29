package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

func PresenceChannel(prefix, userid string) string {
	return fmt.Sprintf("%s.%s", prefix, userid)
}

func PresenceChannelAll(prefix string) string {
	return fmt.Sprintf("%s.>", prefix)
}

func PresenceChannelPrefix(prefix string) string {
	return fmt.Sprintf("%s.", prefix)
}

func UserChannel(prefix, userid, channel string) string {
	return fmt.Sprintf("%s.%s.%s", prefix, userid, channel)
}

func UserChannelAll(prefix, userid string) string {
	return fmt.Sprintf("%s.%s.>", prefix, userid)
}

func UserChannelPrefix(prefix, userid string) string {
	return fmt.Sprintf("%s.%s.", prefix, userid)
}

func ServiceChannel(prefix, channel, userid string) string {
	return fmt.Sprintf("%s.%s.user.%s", prefix, channel, userid)
}

func ServiceChannelAll(prefix, channel string) string {
	return fmt.Sprintf("%s.%s.user.>", prefix, channel)
}

func ServiceChannelPrefix(prefix, channel string) string {
	return fmt.Sprintf("%s.%s.user.", prefix, channel)
}

func (m *router) ws(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	userid := gate.GetCtxUserid(c)
	if userid == "" || len(userid) > 31 {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Message: "User is not authorized",
		})))
		return
	}

	conn, err := c.Websocket()
	if err != nil {
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	presenceChannel := PresenceChannel(m.s.opts.PresenceChannel, userid)
	{
		msg, err := json.Marshal(PresenceEventProps{
			Timestamp: time.Now().Round(0).Unix(),
		})
		if err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithMsg(err, "Failed to marshal ws user presence msg"),
				int(websocket.StatusInternalError),
				"Failed to encode presence msg",
			))
			return
		}
		if err := m.s.events.Publish(presenceChannel, msg); err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithMsg(err, "Failed to publish ws user presence msg"),
				int(websocket.StatusInternalError),
				"Failed to publish presence msg",
			))
			return
		}
	}

	topicPrefix := UserChannelPrefix(m.s.opts.UserSendChannelPrefix, userid)
	sub, err := m.s.events.Subscribe(UserChannelAll(m.s.opts.UserSendChannelPrefix, userid), "", func(topic string, msgdata []byte) {
		b, err := EncodeRcvMsg(strings.TrimPrefix(topic, topicPrefix), msgdata)
		if err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithMsg(err, "Failed to marshal sent json msg"),
				int(websocket.StatusInternalError),
				"Failed to encode msg",
			))
			return
		}
		if err := conn.Write(c.Ctx(), true, b); err != nil {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusAbnormalClosure),
				"Failed to write msg",
			))
			return
		}
	})
	if err != nil {
		conn.CloseError(governor.ErrWS(
			governor.ErrWithMsg(err, "Failed to subscribe to ws user msg channels"),
			int(websocket.StatusInternalError),
			"Failed to subscribe to msgs",
		))
		return
	}
	defer func() {
		if err := sub.Close(); err != nil {
			m.s.logger.Error("Failed to close ws user event subscription", map[string]string{
				"error":      err.Error(),
				"actiontype": "closewsusersub",
			})
		}
	}()

	wg := &sync.WaitGroup{}
	wg.Add(1)
	defer wg.Wait()
	tickCtx, tickCancel := context.WithCancel(c.Ctx())
	defer tickCancel()
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-tickCtx.Done():
				return
			case <-ticker.C:
				msg, err := json.Marshal(PresenceEventProps{
					Timestamp: time.Now().Round(0).Unix(),
				})
				if err != nil {
					conn.CloseError(governor.ErrWS(
						governor.ErrWithMsg(err, "Failed to marshal ws user presence msg"),
						int(websocket.StatusInternalError),
						"Failed to encode presence msg",
					))
					return
				}
				if err := m.s.events.Publish(presenceChannel, msg); err != nil {
					conn.CloseError(governor.ErrWS(
						governor.ErrWithMsg(err, "Failed to publish ws user presence msg"),
						int(websocket.StatusInternalError),
						"Failed to publish presence msg",
					))
					return
				}
			}
		}
	}()

	for {
		t, b, err := conn.Read(c.Ctx())
		if err != nil {
			return
		}
		if !t {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusUnsupportedData),
				"Invalid msg type binary",
			))
			return
		}
		channel, msg, err := DecodeRcvMsg(b)
		if err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithKind(err, governor.ErrorUser{}, "Failed to decode received msg"),
				int(websocket.StatusUnsupportedData),
				"Invalid msg format",
			))
			return
		}
		if channel == "" || len(channel) > 127 {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusUnsupportedData),
				"Invalid msg channel",
			))
			return
		}
		if err := m.s.events.Publish(ServiceChannel(m.s.opts.UserRcvChannelPrefix, channel, userid), msg); err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithMsg(err, "Failed to publish ws user rcv msg"),
				int(websocket.StatusInternalError),
				"Failed to publish msg",
			))
			return
		}
		select {
		case <-c.Ctx().Done():
			return
		case <-time.After(64 * time.Millisecond):
		}
	}
}

func (m *router) echo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	conn, err := c.Websocket()
	if err != nil {
		m.s.logger.Debug("Failed to accept WS conn", map[string]string{
			"value": err.Error(),
		})
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	for {
		t, b, err := conn.Read(c.Ctx())
		if err != nil {
			m.s.logger.Debug("WS conn closed", map[string]string{
				"value": err.Error(),
			})
			return
		}
		if !t {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusUnsupportedData),
				"Invalid msg type binary",
			))
			return
		}
		channel, msg, err := DecodeRcvMsg(b)
		if err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithKind(err, governor.ErrorUser{}, "Failed to decode received msg"),
				int(websocket.StatusUnsupportedData),
				"Invalid msg format",
			))
			return
		}
		if channel != "echo" {
			conn.CloseError(governor.ErrWS(
				governor.NewError(governor.ErrOptUser),
				int(websocket.StatusUnsupportedData),
				"Invalid msg channel",
			))
			return
		}
		res, err := EncodeRcvMsg(channel, msg)
		if err != nil {
			conn.CloseError(governor.ErrWS(
				governor.ErrWithMsg(err, "Failed to marshal sent json msg"),
				int(websocket.StatusInternalError),
				"Failed to encode msg",
			))
			return
		}
		if err := conn.Write(c.Ctx(), true, res); err != nil {
			m.s.logger.Debug("WS conn closed", map[string]string{
				"value": err.Error(),
			})
			return
		}
	}
}

func (m *router) mountRoutes(r governor.Router) {
	scopeWS := m.s.scopens + ".ws"
	r.Any("", m.ws, gate.User(m.s.gate, scopeWS))
	r.Any("/echo", m.echo, gate.Member(m.s.gate, m.s.rolens, scopeWS))
}
