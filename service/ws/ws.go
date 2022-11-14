package ws

import (
	"context"
	"encoding/json"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// PresenceHandlerFunc is a type alias for a presence handler func
	PresenceHandlerFunc = func(ctx context.Context, props PresenceEventProps) error
	// HandlerFunc is a type alias for a handler func
	HandlerFunc = func(ctx context.Context, subject string, userid string, msgdata []byte) error
	// WS is a service for managing websocket connections
	WS interface {
		WatchPresence(location, group string, handler PresenceHandlerFunc) *pubsub.Watcher
		Watch(subject, group string, handler HandlerFunc) *pubsub.Watcher
		Publish(ctx context.Context, userid string, channel string, v interface{}) error
	}

	Service struct {
		pubsub      pubsub.Pubsub
		ratelimiter ratelimit.Ratelimiter
		gate        gate.Gate
		config      governor.ConfigReader
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
	return New(
		pubsub.GetCtxPubsub(inj),
		ratelimit.GetCtxRatelimiter(inj),
		gate.GetCtxGate(inj),
	)
}

// New creates a new WS service
func New(ps pubsub.Pubsub, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
	return &Service{
		pubsub:      ps,
		ratelimiter: ratelimiter,
		gate:        g,
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxWS(inj, s)
	s.rolens = "gov." + r.Name()
	s.scopens = "gov." + r.Name()
	s.channelns = "gov." + r.Name()
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

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

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
		return "", nil, kerrors.WithMsg(err, "Malformed response msg")
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
		return nil, kerrors.WithMsg(err, "Failed to encode request msg")
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

func (s *Service) WatchPresence(location, group string, handler PresenceHandlerFunc) *pubsub.Watcher {
	presenceChannel := presenceChannelName(s.opts.PresenceChannel, location)
	return pubsub.NewWatcher(s.pubsub, s.log.Logger, presenceChannel, group, pubsub.HandlerFunc(func(ctx context.Context, m pubsub.Msg) error {
		var props PresenceEventProps
		if err := kjson.Unmarshal(m.Data, &props); err != nil {
			return kerrors.WithMsg(err, "Invalid presence message")
		}
		return handler(ctx, props)
	}), s.config.Config().Instance)
}

func (s *Service) Watch(subject, group string, handler HandlerFunc) *pubsub.Watcher {
	svcChannel := serviceChannelName(s.opts.UserRcvChannelPrefix, subject)
	return pubsub.NewWatcher(s.pubsub, s.log.Logger, svcChannel, group, pubsub.HandlerFunc(func(ctx context.Context, m pubsub.Msg) error {
		channel, userid, v, err := decodeReqMsg(m.Data)
		if err != nil {
			return kerrors.WithMsg(err, "Failed decoding request message")
		}
		return handler(ctx, channel, userid, v)
	}), s.config.Config().Instance)
}

func (s *Service) Publish(ctx context.Context, userid string, channel string, v interface{}) error {
	b, err := encodeResMsg(channel, v)
	if err != nil {
		return err
	}
	if err := s.pubsub.Publish(ctx, userChannelName(s.opts.UserSendChannelPrefix, userid), b); err != nil {
		return kerrors.WithMsg(err, "Failed to publish to user channel")
	}
	return nil
}
