package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/ktime"
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
	msg, err := kjson.Marshal(PresenceEventProps{
		Timestamp: time.Now().Round(0).Unix(),
		Userid:    userid,
		Location:  loc,
	})
	if err != nil {
		return kerrors.WithMsg(err, "Failed to encode presence msg")
	}
	if err := s.s.pubsub.Publish(ctx, presenceChannelName(s.s.opts.PresenceChannel, loc), msg); err != nil {
		return kerrors.WithMsg(err, "Failed to publish presence msg")
	}
	return nil
}

func (s *router) ws(c *governor.Context) {
	userid := gate.GetCtxUserid(c)

	conn, err := c.Websocket([]string{governor.WSProtocolVersion})
	if err != nil {
		s.s.log.WarnErr(c.Ctx(), kerrors.WithMsg(err, "Failed to accept WS conn upgrade"))
		return
	}
	if conn.Subprotocol() != governor.WSProtocolVersion {
		conn.CloseError(governor.ErrWS(nil, int(websocket.StatusPolicyViolation), "Invalid ws subprotocol"))
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	presenceLocation := ""

	if err := s.sendPresenceUpdate(c.Ctx(), userid, presenceLocation); err != nil {
		s.s.log.Err(c.Ctx(), err)
	}

	wg := &sync.WaitGroup{}
	defer wg.Wait()

	ctx, cancel := context.WithCancel(c.Ctx())
	defer cancel()

	subSuccess := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		userChannel := userChannelName(s.s.opts.UserSendChannelPrefix, userid)
		first := true
		delay := 250 * time.Millisecond
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			func() {
				sub, err := s.s.pubsub.Subscribe(ctx, userChannel, "")
				if err != nil {
					s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to subscribe to ws user msg channels"))
					if err := ktime.After(ctx, delay); err != nil {
						return
					}
					delay = min(delay*2, 15*time.Second)
					return
				}
				defer func() {
					if err := sub.Close(ctx); err != nil {
						s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close ws user event subscription"))
					}
				}()
				delay = 250 * time.Millisecond
				if first {
					first = false
					close(subSuccess)
				}
				for {
					m, err := sub.ReadMsg(ctx)
					if err != nil {
						if errors.Is(err, context.DeadlineExceeded) {
							continue
						}
						if errors.Is(err, pubsub.ErrClientClosed) {
							return
						}
						s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed reading message"))
						if err := ktime.After(ctx, delay); err != nil {
							return
						}
						delay = min(delay*2, 15*time.Second)
						continue
					}
					channel, _, err := decodeResMsg(m.Data)
					if err != nil {
						s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed decoding message"))
						continue
					}
					if channel == "" {
						s.s.log.Err(ctx, kerrors.WithMsg(nil, "Malformed sent message"))
						continue
					}
					if err := conn.Write(ctx, true, m.Data); err != nil {
						conn.CloseError(err)
						cancel()
						return
					}
					return
				}
			}()
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
					s.s.log.Err(ctx, err)
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
		isText, b, err := conn.Read(ctx)
		if err != nil {
			conn.CloseError(err)
			return
		}
		if !isText {
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
			var o ctlOps
			if err := kjson.Unmarshal(msg, &o); err != nil {
				conn.CloseError(governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Invalid ctl op format"))
				return
			}
			origLocation := presenceLocation
			for _, i := range o.Ops {
				switch i.Op {
				case ctlOpLoc:
					{
						var args ctlLocOp
						if err := kjson.Unmarshal(i.Args, &args); err != nil {
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
					s.s.log.Err(ctx, err)
				}
			}
		} else {
			if b, err := encodeReqMsg(channel, userid, msg); err != nil {
				s.s.log.Err(ctx, err)
			} else {
				if err := s.s.pubsub.Publish(ctx, serviceChannelName(s.s.opts.UserRcvChannelPrefix, channel), b); err != nil {
					s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish request msg"))
				}
			}
		}
		if err := ktime.After(ctx, 64*time.Millisecond); err != nil {
			return
		}
	}
}

func (s *router) echo(c *governor.Context) {
	conn, err := c.Websocket([]string{governor.WSProtocolVersion})
	if err != nil {
		s.s.log.WarnErr(c.Ctx(), kerrors.WithMsg(err, "Failed to accept WS conn upgrade"))
		return
	}
	if conn.Subprotocol() != governor.WSProtocolVersion {
		conn.CloseError(governor.ErrWS(nil, int(websocket.StatusPolicyViolation), "Invalid ws subprotocol"))
		return
	}
	defer conn.Close(int(websocket.StatusInternalError), "Internal error")

	for {
		isText, b, err := conn.Read(c.Ctx())
		if err != nil {
			conn.CloseError(err)
			return
		}
		if !isText {
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
	m.AnyCtx("", s.ws, gate.User(s.s.gate, scopeWS), s.rt)
	m.AnyCtx("/echo", s.echo, gate.Member(s.s.gate, s.s.rolens, scopeWS), s.rt)
}
