package ws

import (
	"context"
	"encoding/json"

	"nhooyr.io/websocket"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/kerrors"
)

type (
	// PresenceWorkerFunc is a type alias for a presence worker func
	PresenceWorkerFunc = func(ctx context.Context, props PresenceEventProps)
	// WorkerFunc is a type alias for a worker func
	WorkerFunc = func(ctx context.Context, topic string, userid string, msgdata []byte)
	// WS is a service for managing websocket connections
	WS interface {
		SubscribePresence(location, group string, worker PresenceWorkerFunc) (events.Subscription, error)
		Subscribe(channel, group string, worker WorkerFunc) (events.Subscription, error)
		Publish(ctx context.Context, userid string, channel string, v interface{}) error
	}

	// Service is the public interface for the websocket service
	Service interface {
		governor.Service
		WS
	}

	service struct {
		events    events.Events
		gate      gate.Gate
		logger    governor.Logger
		rolens    string
		scopens   string
		channelns string
		opts      svcOpts
	}

	router struct {
		s *service
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
func NewCtx(inj governor.Injector) Service {
	ev := events.GetCtxEvents(inj)
	g := gate.GetCtxGate(inj)
	return New(ev, g)
}

// New creates a new WS service
func New(ev events.Events, g gate.Gate) Service {
	return &service{
		events: ev,
		gate:   g,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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

func (s *service) router() *router {
	return &router{
		s: s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	sr := s.router()
	sr.mountRoutes(m)
	l.Info("Mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

func encodeResMsg(channel string, v interface{}) ([]byte, error) {
	b, err := json.Marshal(responseMsg{
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
	if err := json.Unmarshal(b, &m); err != nil {
		return "", nil, governor.ErrWS(err, int(websocket.StatusInternalError), "Malformed response msg")
	}
	return m.Channel, m.Value, nil
}

func decodeClientReqMsg(b []byte) (string, []byte, error) {
	var m clientRequestMsgBytes
	if err := json.Unmarshal(b, &m); err != nil {
		return "", nil, governor.ErrWS(err, int(websocket.StatusUnsupportedData), "Malformed request msg")
	}
	return m.Channel, m.Value, nil
}

func encodeReqMsg(channel string, userid string, v []byte) ([]byte, error) {
	b, err := json.Marshal(requestMsgBytes{
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
	if err := json.Unmarshal(b, &m); err != nil {
		return "", "", nil, kerrors.WithMsg(err, "Failed to decode request msg")
	}
	return m.Channel, m.Userid, m.Value, nil
}

func (s *service) SubscribePresence(location, group string, worker PresenceWorkerFunc) (events.Subscription, error) {
	presenceChannel := presenceChannelName(s.opts.PresenceChannel, location)
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": presenceChannel,
		"group":   group,
	})
	sub, err := s.events.Subscribe(presenceChannel, group, func(ctx context.Context, topic string, msgdata []byte) {
		var props PresenceEventProps
		if err := json.Unmarshal(msgdata, &props); err != nil {
			l.Error("Invalid presence message", map[string]string{
				"error":      err.Error(),
				"actiontype": "ws_decode_presence",
			})
			return
		}
		worker(ctx, props)
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to presence channel")
	}
	return sub, nil
}

func (s *service) Subscribe(channel, group string, worker WorkerFunc) (events.Subscription, error) {
	if channel == "" {
		return nil, kerrors.WithMsg(nil, "Channel pattern may not be empty")
	}
	svcChannel := serviceChannelName(s.opts.UserRcvChannelPrefix, channel)
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": svcChannel,
		"group":   group,
	})
	sub, err := s.events.Subscribe(svcChannel, group, func(ctx context.Context, topic string, msgdata []byte) {
		channel, userid, v, err := decodeReqMsg(msgdata)
		if err != nil {
			l.Error("Failed decoding request message", map[string]string{
				"error":      err.Error(),
				"actiontype": "ws_decode_req",
			})
			return
		}
		worker(ctx, channel, userid, v)
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to presence channel")
	}
	return sub, nil
}

func (s *service) Publish(ctx context.Context, userid string, channel string, v interface{}) error {
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
