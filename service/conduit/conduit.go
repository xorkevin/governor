package conduit

import (
	"context"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/chat/model"
	friendmodel "xorkevin.dev/governor/service/conduit/friend/model"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
)

type (
	// Conduit is a service for messaging
	Conduit interface {
	}

	// Service is the public interface for the conduit service
	Service interface {
		governor.Service
		Conduit
	}

	service struct {
		friends  friendmodel.Repo
		repo     model.Repo
		users    user.Users
		events   events.Events
		gate     gate.Gate
		logger   governor.Logger
		scopens  string
		streamns string
		useropts user.Opts
	}

	router struct {
		s *service
	}

	ctxKeyConduit struct{}
)

// GetCtxConduit returns a Conduit service from the context
func GetCtxCourier(inj governor.Injector) Conduit {
	v := inj.Get(ctxKeyConduit{})
	if v == nil {
		return nil
	}
	return v.(Conduit)
}

// setCtxConduit sets a Conduit service in the context
func setCtxConduit(inj governor.Injector, c Conduit) {
	inj.Set(ctxKeyConduit{}, c)
}

// NewCtx creates a new Conduit service from a context
func NewCtx(inj governor.Injector) Service {
	friends := friendmodel.GetCtxRepo(inj)
	repo := model.GetCtxRepo(inj)
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	g := gate.GetCtxGate(inj)
	useropts := user.GetCtxOpts(inj)
	return New(friends, repo, users, ev, g, useropts)
}

// New creates a new Conduit service
func New(friends friendmodel.Repo, repo model.Repo, users user.Users, ev events.Events, g gate.Gate, useropts user.Opts) Service {
	return &service{
		friends:  friends,
		repo:     repo,
		users:    users,
		events:   ev,
		gate:     g,
		useropts: useropts,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxConduit(inj, s)
	s.scopens = name
	s.streamns = strings.ToUpper(name)
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
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.friends.Setup(); err != nil {
		return err
	}
	l.Info("Created conduit friend table", nil)
	if err := s.repo.Setup(); err != nil {
		return err
	}
	l.Info("Created conduit chat tables", nil)

	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(s.useropts.StreamName, s.useropts.CreateChannel, s.streamns+"_WORKER_CREATE", s.UserCreateHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user create queue")
	}
	l.Info("Subscribed to user create queue", nil)

	if _, err := s.events.StreamSubscribe(s.useropts.StreamName, s.useropts.DeleteChannel, s.streamns+"_WORKER_DELETE", s.UserDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)

	if _, err := s.events.StreamSubscribe(s.useropts.StreamName, s.useropts.UpdateChannel, s.streamns+"_WORKER_UPDATE", s.UserUpdateHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user update queue")
	}
	l.Info("Subscribed to user update queue", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// UserCreateHook creates a new user name
func (s *service) UserCreateHook(pinger events.Pinger, msgdata []byte) error {
	props, err := user.DecodeNewUserProps(msgdata)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateUsername(props.Userid, props.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat user name")
	}
	return nil
}

// UserDeleteHook deletes a user name
func (s *service) UserDeleteHook(pinger events.Pinger, msgdata []byte) error {
	props, err := user.DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteUser(props.Userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user from chats")
	}
	return nil
}

// UserUpdateHook updates a user name
func (s *service) UserUpdateHook(pinger events.Pinger, msgdata []byte) error {
	props, err := user.DecodeUpdateUserProps(msgdata)
	if err != nil {
		return err
	}
	if err := s.repo.UpdateUsername(props.Userid, props.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat user name")
	}
	return nil
}
