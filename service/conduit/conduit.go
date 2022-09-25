package conduit

import (
	"context"
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
	"xorkevin.dev/governor/service/events/sysevent"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/ws"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Conduit is a service for messaging
	Conduit interface {
	}

	Service struct {
		friends            friendmodel.Repo
		invitations        invitationmodel.Repo
		dms                dmmodel.Repo
		gdms               gdmmodel.Repo
		servers            servermodel.Repo
		msgs               msgmodel.Repo
		kvpresence         kvstore.KVStore
		users              user.Users
		pubsub             pubsub.Pubsub
		events             events.Events
		ws                 ws.WS
		ratelimiter        ratelimit.Ratelimiter
		gate               gate.Gate
		instance           string
		log                *klog.LevelLogger
		scopens            string
		channelns          string
		streamns           string
		opts               svcOpts
		streamsize         int64
		eventsize          int32
		invitationDuration time.Duration
		gcDuration         time.Duration
		syschannels        governor.SysChannels
		wg                 *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
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
		DMMsgChannel         string
		DMSettingsChannel    string
		GDMMsgChannel        string
		GDMSettingsChannel   string
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
func NewCtx(inj governor.Injector) *Service {
	return New(
		friendmodel.GetCtxRepo(inj),
		invitationmodel.GetCtxRepo(inj),
		dmmodel.GetCtxRepo(inj),
		gdmmodel.GetCtxRepo(inj),
		msgmodel.GetCtxRepo(inj),
		kvstore.GetCtxKVStore(inj),
		user.GetCtxUsers(inj),
		pubsub.GetCtxPubsub(inj),
		events.GetCtxEvents(inj),
		ws.GetCtxWS(inj),
		ratelimit.GetCtxRatelimiter(inj),
		gate.GetCtxGate(inj),
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
	ps pubsub.Pubsub,
	ev events.Events,
	wss ws.WS,
	ratelimiter ratelimit.Ratelimiter,
	g gate.Gate,
) *Service {
	return &Service{
		friends:     friends,
		invitations: invitations,
		dms:         dms,
		gdms:        gdms,
		msgs:        msgs,
		kvpresence:  kv.Subtree("presence"),
		users:       users,
		pubsub:      ps,
		events:      ev,
		ws:          wss,
		ratelimiter: ratelimiter,
		gate:        g,
		wg:          ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
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
		DMMsgChannel:         name + ".chat.dm.msg",
		DMSettingsChannel:    name + ".chat.dm.settings",
		GDMMsgChannel:        name + ".chat.gdm.msg",
		GDMSettingsChannel:   name + ".chat.gdm.settings",
	}

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("invitationduration", "72h")
	r.SetDefault("gcduration", "72h")
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.instance = c.Instance

	var err error
	s.invitationDuration, err = r.GetDuration("invitationduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse friend invitation duration")
	}
	s.gcDuration, err = r.GetDuration("gcduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse gc duration")
	}

	s.syschannels = c.SysChannels

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"conduit.stream.size":        r.GetStr("streamsize"),
		"conduit.event.size":         r.GetStr("eventsize"),
		"conduit.invitationduration": s.invitationDuration.String(),
	})

	sr := s.router()
	sr.mountRoutes(m)
	s.log.Info(ctx, "Mounted http routes", nil)
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.FriendChannel, s.streamns+"_FRIEND_WORKER", s.friendSubscriber, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to friend queue")
	}
	s.log.Info(ctx, "Subscribed to friend queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.UnfriendChannel, s.streamns+"_UNFRIEND_WORKER", s.unfriendSubscriber, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to unfriend queue")
	}
	s.log.Info(ctx, "Subscribed to unfriend queue", nil)

	if _, err := s.users.StreamSubscribeCreate(s.streamns+"_WORKER_CREATE", s.userCreateHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user create queue")
	}
	s.log.Info(ctx, "Subscribed to user create queue", nil)

	if _, err := s.users.StreamSubscribeDelete(s.streamns+"_WORKER_DELETE", s.userDeleteHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	s.log.Info(ctx, "Subscribed to user delete queue", nil)

	if _, err := s.users.StreamSubscribeUpdate(s.streamns+"_WORKER_UPDATE", s.userUpdateHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user update queue")
	}
	s.log.Info(ctx, "Subscribed to user update queue", nil)

	sysEvents := sysevent.New(s.syschannels, s.pubsub, s.log.Logger)
	s.wg.Add(1)
	go sysEvents.WatchGC(s.streamns+"_WORKER_INVITATION_GC", s.friendInvitationGCHook, s.instance).Watch(ctx, s.wg, 15*time.Second)
	s.log.Info(ctx, "Subscribed to gov sys gc channel", nil)

	s.wg.Add(1)
	go s.ws.WatchPresence(s.channelns+".>", s.streamns+"_WORKER_PRESENCE", s.presenceHandler).Watch(ctx, s.wg, 15*time.Second)
	s.log.Info(ctx, "Subscribed to ws presence channel", nil)

	s.wg.Add(1)
	go s.ws.Watch(s.opts.PresenceQueryChannel, s.streamns+"_PRESENCE_QUERY", s.presenceQueryHandler).Watch(ctx, s.wg, 15*time.Second)
	s.log.Info(ctx, "Subscribed to ws presence query channel", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.friends.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit friend table", nil)
	if err := s.invitations.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit friend invitation table", nil)
	if err := s.dms.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit dm table", nil)
	if err := s.gdms.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit gdm tables", nil)
	if err := s.msgs.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit msg table", nil)
	if err := s.events.InitStream(ctx, s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init conduit stream")
	}
	s.log.Info(ctx, "Created conduit stream", nil)
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func (s *Service) userCreateHook(ctx context.Context, pinger events.Pinger, props user.NewUserProps) error {
	if err := s.friends.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update friends username")
	}
	return nil
}

const (
	chatDeleteBatchSize = 256
)

func (s *Service) userDeleteHook(ctx context.Context, pinger events.Pinger, props user.DeleteUserProps) error {
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

func (s *Service) userUpdateHook(ctx context.Context, pinger events.Pinger, props user.UpdateUserProps) error {
	if err := s.friends.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update friends username")
	}
	return nil
}

func (s *Service) friendInvitationGCHook(ctx context.Context, props sysevent.TimestampProps) error {
	if err := s.invitations.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		return kerrors.WithMsg(err, "Failed to GC friend invitations")
	}
	s.log.Info(ctx, "GC friend invitations", nil)
	return nil
}

func decodeFriendProps(msgdata []byte) (*friendProps, error) {
	m := &friendProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode friend props")
	}
	return m, nil
}

func decodeUnfriendProps(msgdata []byte) (*unfriendProps, error) {
	m := &unfriendProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode unfriend props")
	}
	return m, nil
}
