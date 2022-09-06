package conduit

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	dmmodel "xorkevin.dev/governor/service/conduit/dm/model"
	invitationmodel "xorkevin.dev/governor/service/conduit/friend/invitation/model"
	friendmodel "xorkevin.dev/governor/service/conduit/friend/model"
	gdmmodel "xorkevin.dev/governor/service/conduit/gdm/model"
	msgmodel "xorkevin.dev/governor/service/conduit/msg/model"
	servermodel "xorkevin.dev/governor/service/conduit/server/model"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/ws"
	"xorkevin.dev/kerrors"
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
		gdms           gdmmodel.Repo
		servers        servermodel.Repo
		msgs           msgmodel.Repo
		kvpresence     kvstore.KVStore
		users          user.Users
		events         events.Events
		ws             ws.WS
		gate           gate.Gate
		logger         governor.Logger
		scopens        string
		channelns      string
		streamns       string
		opts           svcOpts
		streamsize     int64
		eventsize      int32
		invitationTime int64
		syschannels    governor.SysChannels
	}

	router struct {
		s *service
	}

	friendProps struct {
		Userid    string `json:"userid"`
		InvitedBy string `json:"invited_by"`
	}

	unfriendProps struct {
		Userid string `json:"userid"`
		Other  string `json:"other"`
	}

	ctxKeyConduit struct{}

	svcOpts struct {
		StreamName           string
		FriendChannel        string
		UnfriendChannel      string
		PresenceQueryChannel string
	}
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
	invitations := invitationmodel.GetCtxRepo(inj)
	dms := dmmodel.GetCtxRepo(inj)
	gdms := gdmmodel.GetCtxRepo(inj)
	msgs := msgmodel.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	wss := ws.GetCtxWS(inj)
	g := gate.GetCtxGate(inj)
	return New(
		friends,
		invitations,
		dms,
		gdms,
		msgs,
		kv,
		users,
		ev,
		wss,
		g,
	)
}

// New creates a new Conduit service
func New(
	friends friendmodel.Repo,
	invitations invitationmodel.Repo,
	dms dmmodel.Repo,
	gdms gdmmodel.Repo,
	msgs msgmodel.Repo,
	kv kvstore.KVStore,
	users user.Users,
	ev events.Events,
	wss ws.WS,
	g gate.Gate,
) Service {
	return &service{
		friends:        friends,
		invitations:    invitations,
		dms:            dms,
		gdms:           gdms,
		msgs:           msgs,
		kvpresence:     kv.Subtree("presence"),
		users:          users,
		events:         ev,
		ws:             wss,
		gate:           g,
		invitationTime: time72h,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxConduit(inj, s)
	s.scopens = "gov." + name
	s.channelns = name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = svcOpts{
		StreamName:           streamname,
		FriendChannel:        streamname + ".friend",
		UnfriendChannel:      streamname + ".unfriend",
		PresenceQueryChannel: name + ".presence",
	}

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
		return kerrors.WithMsg(err, "Failed to parse role invitation time")
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
	if err := s.friends.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created conduit friend table", nil)
	if err := s.invitations.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created conduit friend invitation table", nil)
	if err := s.dms.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created conduit dm table", nil)
	if err := s.gdms.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created conduit gdm tables", nil)
	if err := s.msgs.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created conduit msg table", nil)
	if err := s.events.InitStream(context.Background(), s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init conduit stream")
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
		return kerrors.WithMsg(err, "Failed to subscribe to friend queue")
	}
	l.Info("Subscribed to friend queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.UnfriendChannel, s.streamns+"_UNFRIEND_WORKER", s.unfriendSubscriber, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to unfriend queue")
	}
	l.Info("Subscribed to unfriend queue", nil)

	if _, err := s.users.StreamSubscribeCreate(s.streamns+"_WORKER_CREATE", s.UserCreateHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user create queue")
	}
	l.Info("Subscribed to user create queue", nil)

	if _, err := s.users.StreamSubscribeDelete(s.streamns+"_WORKER_DELETE", s.UserDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)

	if _, err := s.users.StreamSubscribeUpdate(s.streamns+"_WORKER_UPDATE", s.UserUpdateHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user update queue")
	}
	l.Info("Subscribed to user update queue", nil)

	if _, err := s.events.Subscribe(s.syschannels.GC, s.streamns+"_WORKER_INVITATION_GC", s.FriendInvitationGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	l.Info("Subscribed to gov sys gc channel", nil)

	if _, err := s.ws.SubscribePresence(s.channelns+".>", s.streamns+"_WORKER_PRESENCE", s.PresenceHandler); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to ws presence channel")
	}
	l.Info("Subscribed to ws presence channel", nil)

	if _, err := s.ws.Subscribe(s.opts.PresenceQueryChannel, s.streamns+"_PRESENCE_QUERY", s.PresenceQueryHandler); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to ws presence query channel")
	}
	l.Info("Subscribed to ws presence query channel", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// UserCreateHook creates a new user name
func (s *service) UserCreateHook(ctx context.Context, pinger events.Pinger, props user.NewUserProps) error {
	if err := s.friends.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update friends username")
	}
	return nil
}

const (
	chatDeleteBatchSize = 256
)

// UserDeleteHook deletes user associated chats
func (s *service) UserDeleteHook(ctx context.Context, pinger events.Pinger, props user.DeleteUserProps) error {
	if err := s.invitations.DeleteByUser(ctx, props.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user invitations")
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		chatids, err := s.gdms.GetLatest(ctx, props.Userid, 0, chatDeleteBatchSize)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user chats")
		}
		if len(chatids) == 0 {
			break
		}
		for _, i := range chatids {
			if err := s.rmGDMUser(ctx, i, props.Userid); err != nil {
				return kerrors.WithMsg(err, "Failed to delete group chat")
			}
		}
		if len(chatids) < chatDeleteBatchSize {
			break
		}
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		friends, err := s.friends.GetFriends(ctx, props.Userid, "", chatDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user friends")
		}
		if len(friends) == 0 {
			break
		}
		for _, i := range friends {
			if err := s.rmFriend(ctx, props.Userid, i.Userid2); err != nil {
				return kerrors.WithMsg(err, "Failed to remove friend")
			}
		}
		if len(friends) < chatDeleteBatchSize {
			break
		}
	}
	return nil
}

// UserUpdateHook updates a user name
func (s *service) UserUpdateHook(ctx context.Context, pinger events.Pinger, props user.UpdateUserProps) error {
	if err := s.friends.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update friends username")
	}
	return nil
}

func (s *service) FriendInvitationGCHook(ctx context.Context, topic string, msgdata []byte) {
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
	if err := s.invitations.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		l.Error(err.Error(), nil)
		return
	}
	l.Debug("GC friend invitations", nil)
}

func decodeFriendProps(msgdata []byte) (*friendProps, error) {
	m := &friendProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode friend props")
	}
	return m, nil
}

func decodeUnfriendProps(msgdata []byte) (*unfriendProps, error) {
	m := &unfriendProps{}
	if err := json.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode unfriend props")
	}
	return m, nil
}
