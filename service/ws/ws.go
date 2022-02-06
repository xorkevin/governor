package ws

import (
	"context"
	"encoding/json"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user/gate"
)

type (
	// WS is a service for managing websocket connections
	WS interface {
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
		opts      Opts
	}

	router struct {
		s *service
	}

	// rcvMsg is a received ws msg
	rcvMsg struct {
		Channel string          `json:"channel"`
		Value   json.RawMessage `json:"value"`
	}

	// SendMsg is a sent msg
	SendMsg struct {
		Channel string      `json:"channel"`
		Value   interface{} `json:"value"`
	}

	ctxKeyWS struct{}

	Opts struct {
		PresenceChannel       string
		UserSendChannelPrefix string
		UserRcvChannelPrefix  string
	}

	ctxKeyOpts struct{}

	PresenceEventProps struct {
		Timestamp int64  `json:"timestamp"`
		Location  string `json:"location"`
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

func GetCtxOpts(inj governor.Injector) Opts {
	v := inj.Get(ctxKeyOpts{})
	if v == nil {
		return Opts{}
	}
	return v.(Opts)
}

func SetCtxOpts(inj governor.Injector, o Opts) {
	inj.Set(ctxKeyOpts{}, o)
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
	s.opts = Opts{
		PresenceChannel:       s.channelns + ".presence.user",
		UserSendChannelPrefix: s.channelns + ".send.user",
		UserRcvChannelPrefix:  s.channelns + ".rcv",
	}
	SetCtxOpts(inj, s.opts)
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

// decodeRcvMsg unmarshals json encoded received messages into a struct
func decodeRcvMsg(b []byte) (string, []byte, error) {
	m := &rcvMsg{}
	if err := json.Unmarshal(b, m); err != nil {
		return "", nil, governor.ErrWithMsg(err, "Failed to decode received msg")
	}
	return m.Channel, m.Value, nil
}

// encodeRcvMsg marshals received messages to json
func encodeRcvMsg(channel string, v []byte) ([]byte, error) {
	b, err := json.Marshal(rcvMsg{
		Channel: channel,
		Value:   v,
	})
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to encode received msg")
	}
	return b, nil
}
