package ws

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/kerrors"
)

func presenceChannelName(prefix, location string) string {
	if location == "" {
		return prefix
	}
	return prefix + "." + location
}

func userChannelName(prefix, userid string) string {
	return prefix + "." + userid
}

func serviceChannelName(prefix, channel string) string {
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

func (s *router) sendPresenceUpdate(ctx context.Context, userid, loc string) error {
	msg, err := json.Marshal(PresenceEventProps{
		Timestamp: time.Now().Round(0).Unix(),
		Userid:    userid,
		Location:  loc,
	})
	if err != nil {
		return governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to encode presence msg")
	}
	if err := s.s.events.Publish(ctx, presenceChannelName(s.s.opts.PresenceChannel, loc), msg); err != nil {
		return governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to publish presence msg")
	}
	return nil
}

func (s *router) ws(c governor.Context) {
	userid := gate.GetCtxUserid(c)

	conn, err := c.Websocket()
	if err != nil {
		s.s.log.WarnErr(c.Ctx(), kerrors.WithMsg(err, "Failed to accept WS conn upgrade"), nil)
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	presenceLocation := ""

	if err := s.sendPresenceUpdate(c.Ctx(), userid, presenceLocation); err != nil {
		conn.CloseError(err)
		return
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, tickCancel := context.WithCancel(c.Ctx())
	defer tickCancel()

	subSuccess := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		userChannel := userChannelName(s.s.opts.UserSendChannelPrefix, userid)
		var sub events.SyncSubscription
		defer func() {
			if sub != nil {
				if err := sub.Close(); err != nil {
					s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close ws user event subscription"), nil)
				}
			}
		}()
		first := true
		delay := 250 * time.Millisecond
		for {
			k, err := s.s.events.SubscribeSync(ctx, userChannel, "", func(ctx context.Context, topic string, msgdata []byte) error {
				channel, _, err := decodeResMsg(msgdata)
				if err != nil {
					conn.CloseError(err)
					return nil
				}
				if channel == "" {
					conn.CloseError(governor.ErrWS(nil, int(websocket.StatusInternalError), "Malformed sent message"))
					return nil
				}
				if err := conn.Write(ctx, true, msgdata); err != nil {
					conn.CloseError(err)
					return nil
				}
				return nil
			})
			if err != nil {
				s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to subscribe to ws user msg channels"), nil)
				select {
				case <-ctx.Done():
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
					s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close ws user event subscription"), nil)
				}
			}
			select {
			case <-ctx.Done():
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
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.sendPresenceUpdate(ctx, userid, presenceLocation); err != nil {
					conn.CloseError(err)
					return
				}
			}
		}
	}()

	select {
	case <-ctx.Done():
		return
	case <-subSuccess:
	}

	for {
		t, b, err := conn.Read(ctx)
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
				if err := s.sendPresenceUpdate(ctx, userid, presenceLocation); err != nil {
					conn.CloseError(err)
					return
				}
			}
		} else {
			b, err := encodeReqMsg(channel, userid, msg)
			if err != nil {
				conn.CloseError(err)
				return
			}
			if err := s.s.events.Publish(ctx, serviceChannelName(s.s.opts.UserRcvChannelPrefix, channel), b); err != nil {
				conn.CloseError(governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to publish request msg"))
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(64 * time.Millisecond):
		}
	}
}

func (s *router) echo(c governor.Context) {
	conn, err := c.Websocket()
	if err != nil {
		s.s.log.WarnErr(c.Ctx(), kerrors.WithMsg(err, "Failed to accept WS conn upgrade"), nil)
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	for {
		t, b, err := conn.Read(c.Ctx())
		if err != nil {
			conn.CloseError(err)
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

func (s *router) mountRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeWS := s.s.scopens + ".ws"
	m.AnyCtx("", s.ws, gate.User(s.s.gate, scopeWS))
	m.AnyCtx("/echo", s.echo, gate.Member(s.s.gate, s.s.rolens, scopeWS))
}
