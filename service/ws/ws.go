package ws

import (
	"context"
	"encoding/json"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// PresenceWorkerFunc is a type alias for a presence worker func
	PresenceWorkerFunc = func(ctx context.Context, props PresenceEventProps) error
	// WorkerFunc is a type alias for a worker func
	WorkerFunc = func(ctx context.Context, topic string, userid string, msgdata []byte) error
	// WS is a service for managing websocket connections
	WS interface {
		SubscribePresence(location, group string, worker PresenceWorkerFunc) (events.Subscription, error)
		Subscribe(channel, group string, worker WorkerFunc) (events.Subscription, error)
		Publish(ctx context.Context, userid string, channel string, v interface{}) error
	}

	Service struct {
		events      events.Events
		ratelimiter ratelimit.Ratelimiter
		gate        gate.Gate
		log         *klog.LevelLogger
		rolens      string
		scopens     string
		channelns   string
		opts        svcOpts
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}

	ctxKeyWS struct{}

	svcOpts struct {
		PresenceChannel       string
		UserSendChannelPrefix string
		UserRcvChannelPrefix  string
	}

	PresenceEventProps struct {
		Timestamp int64  `json:"timestamp"`
		Userid    string `json:"userid"`
		Location  string `json:"location"`
	}

	// responseMsgBytes is a partially decoded response msg to the client
	responseMsgBytes struct {
		Channel string          `json:"channel"`
		Value   json.RawMessage `json:"value"`
	}

	// responseMsg is a response msg to the client
	responseMsg struct {
		Channel string      `json:"channel"`
		Value   interface{} `json:"value"`
	}

	// clientRequestMsg is a partially decoded request msg from a client for a service
	clientRequestMsgBytes struct {
		Channel string          `json:"channel"`
		Value   json.RawMessage `json:"value"`
	}

	// requestMsgBytes is a partially encoded request msg to a service
	requestMsgBytes struct {
		Channel string          `json:"channel"`
		Userid  string          `json:"userid"`
		Value   json.RawMessage `json:"value"`
	}
)

// GetCtxWS returns a WS service from the context
func GetCtxWS(inj governor.Injector) WS {
	v := inj.Get(ctxKeyWS{})
	if v == nil {
		return nil
	}
	return v.(WS)
}

// setCtxWS sets a WS service in the context
func setCtxWS(inj governor.Injector, w WS) {
	inj.Set(ctxKeyWS{}, w)
}

// NewCtx creates a new WS service from a context
func NewCtx(inj governor.Injector) *Service {
	ev := events.GetCtxEvents(inj)
	g := gate.GetCtxGate(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	return New(ev, ratelimiter, g)
}

// New creates a new WS service
func New(ev events.Events, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
	return &Service{
		events:      ev,
		ratelimiter: ratelimiter,
		gate:        g,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxWS(inj, s)
	s.rolens = "gov." + name
	s.scopens = "gov." + name
	s.channelns = "gov." + name
	s.opts = svcOpts{
		PresenceChannel:       s.channelns + ".presence.loc",
		UserSendChannelPrefix: s.channelns + ".send.user",
		UserRcvChannelPrefix:  s.channelns + ".rcv.svc",
	}
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	sr := s.router()
	sr.mountRoutes(m)
	s.log.Info(ctx, "Mounted http routes", nil)
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func encodeResMsg(channel string, v interface{}) ([]byte, error) {
	b, err := kjson.Marshal(responseMsg{
		Channel: channel,
		Value:   v,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode response msg")
	}
	return b, nil
}

func decodeResMsg(b []byte) (string, []byte, error) {
	var m responseMsgBytes
	if err := kjson.Unmarshal(b, &m); err != nil {
		return "", nil, governor.ErrWS(err, int(websocket.StatusInternalError), "Malformed response msg")
	}
	return m.Channel, m.Value, nil
}

func decodeClientReqMsg(b []byte) (string, []byte, error) {
	var m clientRequestMsgBytes
	if err := kjson.Unmarshal(b, &m); err != nil {
		return "", nil, governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Malformed request msg")
	}
	return m.Channel, m.Value, nil
}

func encodeReqMsg(channel string, userid string, v []byte) ([]byte, error) {
	b, err := kjson.Marshal(requestMsgBytes{
		Channel: channel,
		Userid:  userid,
		Value:   v,
	})
	if err != nil {
		return nil, governor.ErrWS(err, int(websocket.StatusInternalError), "Failed to encode request msg")
	}
	return b, nil
}

func decodeReqMsg(b []byte) (string, string, []byte, error) {
	var m requestMsgBytes
	if err := kjson.Unmarshal(b, &m); err != nil {
		return "", "", nil, kerrors.WithMsg(err, "Failed to decode request msg")
	}
	return m.Channel, m.Userid, m.Value, nil
}

func (s *Service) SubscribePresence(location, group string, worker PresenceWorkerFunc) (events.Subscription, error) {
	presenceChannel := presenceChannelName(s.opts.PresenceChannel, location)
	sub, err := s.events.Subscribe(presenceChannel, group, func(ctx context.Context, topic string, msgdata []byte) error {
		var props PresenceEventProps
		if err := kjson.Unmarshal(msgdata, &props); err != nil {
			return kerrors.WithMsg(err, "Invalid presence message")
		}
		return worker(ctx, props)
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to presence channel")
	}
	return sub, nil
}

func (s *Service) Subscribe(channel, group string, worker WorkerFunc) (events.Subscription, error) {
	if channel == "" {
		return nil, kerrors.WithMsg(nil, "Channel pattern may not be empty")
	}
	svcChannel := serviceChannelName(s.opts.UserRcvChannelPrefix, channel)
	sub, err := s.events.Subscribe(svcChannel, group, func(ctx context.Context, topic string, msgdata []byte) error {
		channel, userid, v, err := decodeReqMsg(msgdata)
		if err != nil {
			return kerrors.WithMsg(err, "Failed decoding request message")
		}
		return worker(ctx, channel, userid, v)
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to presence channel")
	}
	return sub, nil
}

func (s *Service) Publish(ctx context.Context, userid string, channel string, v interface{}) error {
	if userid == "" {
		return kerrors.WithMsg(nil, "Userid may not be empty")
	}
	b, err := encodeResMsg(channel, v)
	if err != nil {
		return err
	}
	if err := s.events.Publish(ctx, userChannelName(s.opts.UserSendChannelPrefix, userid), b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish to user channel")
	}
	return nil
}
