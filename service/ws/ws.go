package ws

import (
	"context"
	"encoding/json"
	"strings"

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
		events   events.Events
		gate     gate.Gate
		logger   governor.Logger
		scopens  string
		streamns string
	}

	router struct {
		s *service
	}

	// RcvMsg is a received ws msg
	RcvMsg struct {
		Channel string          `json:"channel"`
		Value   json.RawMessage `json:"value"`
	}

	// SendMsg is a sent msg
	SendMsg struct {
		Channel string      `json:"channel"`
		Value   interface{} `json:"value"`
	}

	ctxKeyWS struct{}
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
	s.scopens = name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
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

// DecodeRcvMsg unmarshals json encoded received messages into a struct
func DecodeRcvMsg(b []byte) (string, []byte, error) {
	m := &RcvMsg{}
	if err := json.Unmarshal(b, m); err != nil {
		return "", nil, governor.ErrWithMsg(err, "Failed to decode received msg")
	}
	return m.Channel, m.Value, nil
}

// EncodeSendMsg marshals sent messages to json
func EncodeSendMsg(channel string, v interface{}) ([]byte, error) {
	b, err := json.Marshal(SendMsg{
		Channel: channel,
		Value:   v,
	})
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to encode sent msg")
	}
	return b, nil
}
