package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user/gate"
)

func PresenceChannel(prefix, userid string) string {
	return prefix + "." + userid
}

func UserChannel(prefix, userid string) string {
	return prefix + "." + userid
}

func ServiceChannel(prefix, channel string) string {
	return prefix + "." + channel
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
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

func (m *router) sendPresenceUpdate(ctx context.Context, userid, loc string) error {
	msg, err := json.Marshal(PresenceEventProps{
		Timestamp: time.Now().Round(0).Unix(),
		Userid:    userid,
		Location:  loc,
	})
	if err != nil {
		return governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to encode presence msg")
	}
	if err := m.s.events.Publish(ctx, m.s.opts.PresenceChannel, msg); err != nil {
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

	if err := m.sendPresenceUpdate(c.Ctx(), userid, presenceLocation); err != nil {
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
		userChannel := UserChannel(m.s.opts.UserSendChannelPrefix, userid)
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
			k, err := m.s.events.SubscribeSync(tickCtx, userChannel, "", func(ctx context.Context, topic string, msgdata []byte) {
				channel, _, err := decodeResMsg(msgdata)
				if err != nil {
					conn.CloseError(err)
					return
				}
				if channel == "" {
					conn.CloseError(governor.ErrWS(nil, int(websocket.StatusInternalError), "Malformed sent message"))
					return
				}
				if err := conn.Write(ctx, true, msgdata); err != nil {
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
				if err := m.sendPresenceUpdate(tickCtx, userid, presenceLocation); err != nil {
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
		channel, msg, err := decodeClientReqMsg(b)
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
				if err := m.sendPresenceUpdate(tickCtx, userid, presenceLocation); err != nil {
					conn.CloseError(err)
					return
				}
			}
		} else {
			b, err := encodeReqMsg(userid, msg)
			if err != nil {
				conn.CloseError(err)
				return
			}
			if err := m.s.events.Publish(tickCtx, ServiceChannel(m.s.opts.UserRcvChannelPrefix, channel), b); err != nil {
				conn.CloseError(governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to publish request msg"))
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
		channel, _, err := decodeClientReqMsg(b)
		if err != nil {
			conn.CloseError(err)
			return
		}
		if channel != "echo" {
			conn.CloseError(governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid msg channel"))
			return
		}
		if err := conn.Write(c.Ctx(), true, b); err != nil {
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
