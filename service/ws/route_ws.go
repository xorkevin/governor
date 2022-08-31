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
	"xorkevin.dev/governor/service/events"
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

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

// decodeRcvMsg unmarshals json encoded received messages into a struct
func decodeRcvMsg(b []byte) (string, []byte, error) {
	m := &rcvMsg{}
	if err := json.Unmarshal(b, m); err != nil {
		return "", nil, governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Failed to decode received msg")
	}
	return m.Channel, m.Value, nil
}

// encodeSendMsg marshals sent messages to json
func encodeSendMsg(channel string, v []byte) ([]byte, error) {
	b, err := json.Marshal(rcvMsg{
		Channel: channel,
		Value:   v,
	})
	if err != nil {
		return nil, governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to encode sent msg")
	}
	return b, nil
}

const (
	ctlChannel = "_ctl_"
	ctlOpLoc   = "location"
)

type (
	ctlOp struct {
		Op   string          `json:"op"`
		Args json.RawMessage `json:"args"`
	}

	ctlOps struct {
		Ops []ctlOp `json:"ops"`
	}

	ctlLocOp struct {
		Location string `json:"location"`
	}
)

func (m *router) sendPresenceUpdate(ctx context.Context, ch, loc string) error {
	msg, err := json.Marshal(PresenceEventProps{
		Timestamp: time.Now().Round(0).Unix(),
		Location:  loc,
	})
	if err != nil {
		return governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to encode presence msg")
	}
	if err := m.s.events.Publish(ctx, ch, msg); err != nil {
		return governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to publish presence msg")
	}
	return nil
}

func (m *router) ws(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	userid := gate.GetCtxUserid(c)
	if userid == "" || len(userid) > 31 {
		c.WriteError(governor.ErrWithRes(nil, http.StatusUnauthorized, "", "User is not authorized"))
		return
	}

	conn, err := c.Websocket()
	if err != nil {
		m.s.logger.Warn("Failed to accept WS conn upgrade", map[string]string{
			"error":      err.Error(),
			"actiontype": "ws_conn_upgrade",
		})
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	presenceLocation := ""

	presenceChannel := PresenceChannel(m.s.opts.PresenceChannel, userid)
	if err := m.sendPresenceUpdate(c.Ctx(), presenceChannel, presenceLocation); err != nil {
		conn.CloseError(err)
		return
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	tickCtx, tickCancel := context.WithCancel(c.Ctx())
	defer tickCancel()

	subSuccess := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		topicPrefix := UserChannelPrefix(m.s.opts.UserSendChannelPrefix, userid)
		var sub events.SyncSubscription
		defer func() {
			if sub != nil {
				if err := sub.Close(); err != nil {
					m.s.logger.Error("Failed to close ws user event subscription", map[string]string{
						"error":      err.Error(),
						"actiontype": "ws_close_user_sub",
					})
				}
			}
		}()
		first := true
		delay := 250 * time.Millisecond
		for {
			k, err := m.s.events.SubscribeSync(tickCtx, UserChannelAll(m.s.opts.UserSendChannelPrefix, userid), "", func(ctx context.Context, topic string, msgdata []byte) {
				b, err := encodeSendMsg(strings.TrimPrefix(topic, topicPrefix), msgdata)
				if err != nil {
					conn.CloseError(err)
					return
				}
				if err := conn.Write(ctx, true, b); err != nil {
					conn.CloseError(err)
					return
				}
			})
			if err != nil {
				m.s.logger.Error("Failed to subscribe to ws user msg channels", map[string]string{
					"error":      err.Error(),
					"actiontype": "ws_sub_user",
				})
				select {
				case <-tickCtx.Done():
					return
				case <-time.After(delay):
					delay *= min(delay*2, 15*time.Second)
				}
				continue
			}
			sub, k = k, sub
			delay = 250 * time.Millisecond
			if first {
				first = false
				close(subSuccess)
			}
			if k != nil {
				if err := k.Close(); err != nil {
					m.s.logger.Error("Failed to close ws user event subscription", map[string]string{
						"error":      err.Error(),
						"actiontype": "ws_close_user_sub",
					})
				}
			}
			select {
			case <-tickCtx.Done():
				return
			case <-sub.Done():
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-tickCtx.Done():
				return
			case <-ticker.C:
				if err := m.sendPresenceUpdate(tickCtx, presenceChannel, presenceLocation); err != nil {
					conn.CloseError(err)
					return
				}
			}
		}
	}()

	select {
	case <-tickCtx.Done():
		return
	case <-subSuccess:
	}

	for {
		t, b, err := conn.Read(tickCtx)
		if err != nil {
			conn.CloseError(err)
			return
		}
		if !t {
			conn.CloseError(governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg type binary"))
			return
		}
		channel, msg, err := decodeRcvMsg(b)
		if err != nil {
			conn.CloseError(err)
			return
		}
		if channel == "" || len(channel) > 127 {
			conn.CloseError(governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg channel"))
			return
		}
		if channel == ctlChannel {
			o := &ctlOps{}
			if err := json.Unmarshal(msg, o); err != nil {
				conn.CloseError(governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Invalid ctl op format"))
				return
			}
			origLocation := presenceLocation
			for _, i := range o.Ops {
				switch i.Op {
				case ctlOpLoc:
					{
						args := &ctlLocOp{}
						if err := json.Unmarshal(i.Args, args); err != nil {
							conn.CloseError(governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Invalid ctl loc op format"))
							return
						}
						if len(args.Location) > 127 {
							conn.CloseError(governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid location"))
							return
						}
						presenceLocation = args.Location
					}
				}
			}
			if presenceLocation != origLocation {
				if err := m.sendPresenceUpdate(tickCtx, presenceChannel, presenceLocation); err != nil {
					conn.CloseError(err)
					return
				}
			}
		} else {
			if err := m.s.events.Publish(tickCtx, ServiceChannel(m.s.opts.UserRcvChannelPrefix, channel, userid), msg); err != nil {
				conn.CloseError(governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to publish service msg"))
				return
			}
		}
		select {
		case <-tickCtx.Done():
			return
		case <-time.After(64 * time.Millisecond):
		}
	}
}

func (m *router) echo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	conn, err := c.Websocket()
	if err != nil {
		m.s.logger.Warn("Failed to accept WS conn upgrade", map[string]string{
			"error":      err.Error(),
			"actiontype": "ws_conn_upgrade",
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
			conn.CloseError(governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg type binary"))
			return
		}
		channel, msg, err := decodeRcvMsg(b)
		if err != nil {
			conn.CloseError(err)
			return
		}
		if channel != "echo" {
			conn.CloseError(governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg channel"))
			return
		}
		res, err := encodeSendMsg(channel, msg)
		if err != nil {
			conn.CloseError(err)
			return
		}
		if err := conn.Write(c.Ctx(), true, res); err != nil {
			conn.CloseError(err)
			return
		}
	}
}

func (m *router) mountRoutes(r governor.Router) {
	scopeWS := m.s.scopens + ".ws"
	r.Any("", m.ws, gate.User(m.s.gate, scopeWS))
	r.Any("/echo", m.echo, gate.Member(m.s.gate, m.s.rolens, scopeWS))
}
