package conduit

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/chat/model"
	dmmodel "xorkevin.dev/governor/service/conduit/dm/model"
	invitationmodel "xorkevin.dev/governor/service/conduit/friend/invitation/model"
	friendmodel "xorkevin.dev/governor/service/conduit/friend/model"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
	time72h int64 = time24h * 3
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
		friends        friendmodel.Repo
		invitations    invitationmodel.Repo
		dms            dmmodel.Repo
		repo           model.Repo
		users          user.Users
		events         events.Events
		gate           gate.Gate
		logger         governor.Logger
		scopens        string
		streamns       string
		opts           Opts
		streamsize     int64
		eventsize      int32
		invitationTime int64
		useropts       user.Opts
		syschannels    governor.SysChannels
	}

	router struct {
		s *service
	}

	// FriendProps are properties of a friend event
	FriendProps struct {
		Userid    string `json:"userid"`
		InvitedBy string `json:"invited_by"`
	}

	// UnfriendProps are properties of an unfriend event
	UnfriendProps struct {
		Userid string `json:"userid"`
		Other  string `json:"other"`
	}

	ctxKeyConduit struct{}

	Opts struct {
		StreamName      string
		FriendChannel   string
		UnfriendChannel string
	}

	ctxKeyOpts struct{}
)

// GetCtxOpts returns conduit Opts from the context
func GetCtxOpts(inj governor.Injector) Opts {
	v := inj.Get(ctxKeyOpts{})
	if v == nil {
		return Opts{}
	}
	return v.(Opts)
}

// SetCtxOpts sets conduit Opts in the context
func SetCtxOpts(inj governor.Injector, o Opts) {
	inj.Set(ctxKeyOpts{}, o)
}

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
	invitations := invitationmodel.GetCtxRepo(inj)
	dms := dmmodel.GetCtxRepo(inj)
	repo := model.GetCtxRepo(inj)
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	g := gate.GetCtxGate(inj)
	useropts := user.GetCtxOpts(inj)
	return New(friends, invitations, dms, repo, users, ev, g, useropts)
}

// New creates a new Conduit service
func New(friends friendmodel.Repo, invitations invitationmodel.Repo, dms dmmodel.Repo, repo model.Repo, users user.Users, ev events.Events, g gate.Gate, useropts user.Opts) Service {
	return &service{
		friends:        friends,
		invitations:    invitations,
		dms:            dms,
		repo:           repo,
		users:          users,
		events:         ev,
		gate:           g,
		invitationTime: time72h,
		useropts:       useropts,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxConduit(inj, s)
	s.scopens = name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = Opts{
		StreamName:      streamname,
		FriendChannel:   streamname + ".friend",
		UnfriendChannel: streamname + ".unfriend",
	}
	SetCtxOpts(inj, s.opts)

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("invitationtime", "72h")
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

	if t, err := time.ParseDuration(r.GetStr("invitationtime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse role invitation time")
	} else {
		s.invitationTime = int64(t / time.Second)
	}

	s.syschannels = c.SysChannels

	l.Info("Loaded config", map[string]string{
		"stream size (bytes)": r.GetStr("streamsize"),
		"event size (bytes)":  r.GetStr("eventsize"),
		"invitationtime (s)":  strconv.FormatInt(s.invitationTime, 10),
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
	if err := s.invitations.Setup(); err != nil {
		return err
	}
	l.Info("Created conduit friend invitation table", nil)
	if err := s.dms.Setup(); err != nil {
		return err
	}
	l.Info("Created conduit dm table", nil)
	if err := s.repo.Setup(); err != nil {
		return err
	}
	l.Info("Created conduit chat tables", nil)
	if err := s.events.InitStream(s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to init conduit stream")
	}
	l.Info("Created conduit stream", nil)
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.FriendChannel, s.streamns+"_FRIEND_WORKER", s.friendSubscriber, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to friend queue")
	}
	l.Info("Subscribed to friend queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.UnfriendChannel, s.streamns+"_UNFRIEND_WORKER", s.unfriendSubscriber, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to unfriend queue")
	}
	l.Info("Subscribed to unfriend queue", nil)

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

	if _, err := s.events.Subscribe(s.syschannels.GC, s.streamns+"_WORKER_INVITATION_GC", s.FriendInvitationGCHook); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	l.Info("Subscribed to gov sys gc channel", nil)

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
	if err := s.friends.UpdateUsername(props.Userid, props.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to update friends username")
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
	if err := s.friends.DeleteUser(props.Userid); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete user friends")
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
	if err := s.friends.UpdateUsername(props.Userid, props.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to update friends username")
	}
	if err := s.repo.UpdateUsername(props.Userid, props.Username); err != nil {
		return governor.ErrWithMsg(err, "Failed to update chat user name")
	}
	return nil
}

func (s *service) FriendInvitationGCHook(msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": s.syschannels.GC,
		"group":   s.streamns + "_WORKER_INVITATION_GC",
	})
	props, err := governor.DecodeSysEventTimestampProps(msgdata)
	if err != nil {
		l.Error(err.Error(), nil)
		return
	}
	if err := s.invitations.DeleteBefore(props.Timestamp - time72h); err != nil {
		l.Error(err.Error(), nil)
		return
	}
	l.Debug("GC friend invitations", nil)
}

// DecodeFriendProps unmarshals json encoded friend props into a struct
func DecodeFriendProps(msgdata []byte) (*FriendProps, error) {
	m := &FriendProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to decode friend props")
	}
	return m, nil
}

// DecodeUnfriendProps unmarshals json encoded unfriend props into a struct
func DecodeUnfriendProps(msgdata []byte) (*UnfriendProps, error) {
	m := &UnfriendProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to decode unfriend props")
	}
	return m, nil
}
