package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
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

	var wg sync.WaitGroup
	defer wg.Wait()

	ctx, cancel := context.WithCancel(c.Ctx())
	defer cancel()

	{
		userChannel := userChannelName(s.s.opts.UserSendChannelPrefix, userid)
		sub, err := s.s.pubsub.Subscribe(ctx, userChannel, "")
		if err != nil {
			conn.CloseError(governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to subscribe to ws user msg channels"))
			return
		}
		wg.Add(1)
		go s.consumeSend(ctx, cancel, &wg, conn, sub)
	}

	var presenceLocation atomic.Pointer[string]
	{
		var emptystr string
		presenceLocation.Store(&emptystr)
	}

	wg.Add(1)
	go s.presenceHeartbeat(ctx, &wg, userid, &presenceLocation)

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
			if err := s.handleCtlMsg(ctx, conn, msg, userid, &presenceLocation); err != nil {
				conn.CloseError(err)
				return
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

func (s *router) consumeSend(ctx context.Context, cancel context.CancelFunc, wg *sync.WaitGroup, conn *governor.Websocket, sub pubsub.Subscription) {
	defer wg.Done()
	defer func() {
		if err := sub.Close(ctx); err != nil {
			s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed to close ws user event subscription"))
		}
	}()

	delay := 250 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		m, err := sub.ReadMsg(ctx)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if errors.Is(err, pubsub.ErrClientClosed) {
				conn.CloseError(governor.ErrWS(nil, int(websocket.StatusInternalError), "Subscription to user msg channels closed"))
				cancel()
				return
			}
			s.s.log.Err(ctx, kerrors.WithMsg(err, "Failed reading message"))
			if err := ktime.After(ctx, delay); err != nil {
				return
			}
			delay = min(delay*2, 15*time.Second)
			continue
		}
		delay = 250 * time.Millisecond
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
	}
}

func (s *router) presenceHeartbeat(ctx context.Context, wg *sync.WaitGroup, userid string, loc *atomic.Pointer[string]) {
	defer wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	if err := s.sendPresenceUpdate(ctx, userid, *loc.Load()); err != nil {
		s.s.log.Err(ctx, err)
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sendPresenceUpdate(ctx, userid, *loc.Load()); err != nil {
				s.s.log.Err(ctx, err)
			}
		}
	}
}

func (s *router) handleCtlMsg(ctx context.Context, conn *governor.Websocket, msg []byte, userid string, loc *atomic.Pointer[string]) error {
	var o ctlOps
	if err := kjson.Unmarshal(msg, &o); err != nil {
		return governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Invalid ctl op format")
	}
	changedLocation := false
	curLocation := *loc.Load()
	nextLocation := ""
	for _, i := range o.Ops {
		switch i.Op {
		case ctlOpLoc:
			{
				var args ctlLocOp
				if err := kjson.Unmarshal(i.Args, &args); err != nil {
					return governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Invalid ctl loc op format")
				}
				if len(args.Location) > 127 {
					return governor.ErrWS(nil, int(websocket.StatusUnsupportedData), "Invalid location")
				}
				if args.Location != curLocation {
					changedLocation = true
					nextLocation = args.Location
				}
			}
		}
	}
	if changedLocation {
		loc.Store(&nextLocation)
		if err := s.sendPresenceUpdate(ctx, userid, nextLocation); err != nil {
			s.s.log.Err(ctx, err)
		}
	}
	return nil
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
