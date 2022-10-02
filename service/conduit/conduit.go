package conduit

import (
	"context"
	"encoding/json"
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

const (
	conduitEventKindFriend   = "friend"
	conduitEventKindUnfriend = "unfriend"
)

type (
	conduitEventDec struct {
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}

	conduitEventEnc struct {
		Kind    string      `json:"kind"`
		Payload interface{} `json:"payload"`
	}

	conduitEvent struct {
		Kind     string
		Friend   friendProps
		Unfriend unfriendProps
	}

	friendProps struct {
		Userid    string `json:"userid"`
		InvitedBy string `json:"invited_by"`
	}

	unfriendProps struct {
		Userid string `json:"userid"`
		Other  string `json:"other"`
	}

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
		streamconduit      string
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

	ctxKeyConduit struct{}

	svcOpts struct {
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
	s.streamns = name
	s.streamconduit = name
	s.opts = svcOpts{
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
	s.wg.Add(1)
	go events.NewWatcher(
		s.events,
		s.log.Logger,
		s.streamconduit,
		s.streamns+".worker",
		events.ConsumerOpts{},
		events.HandlerFunc(s.conduitEventHandler),
		nil,
		0,
		s.instance,
	).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to conduit stream", nil)

	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream", nil)

	sysEvents := sysevent.New(s.syschannels, s.pubsub, s.log.Logger)
	s.wg.Add(1)
	go sysEvents.WatchGC(s.streamns+"_WORKER_INVITATION_GC", s.friendInvitationGCHook, s.instance).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to gov sys gc channel", nil)

	s.wg.Add(1)
	go s.ws.WatchPresence(s.channelns+".>", s.streamns+"_WORKER_PRESENCE", s.presenceHandler).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to ws presence channel", nil)

	s.wg.Add(1)
	go s.ws.Watch(s.opts.PresenceQueryChannel, s.streamns+"_PRESENCE_QUERY", s.presenceQueryHandler).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to ws conduit presence query channel", nil)

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
	if err := s.events.InitStream(ctx, s.streamconduit, events.StreamOpts{
		Partitions:     16,
		Replicas:       1,
		ReplicaQuorum:  1,
		RetentionAge:   30 * 24 * time.Hour,
		RetentionBytes: int(s.streamsize),
		MaxMsgBytes:    int(s.eventsize),
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init conduit stream")
	}
	s.log.Info(ctx, "Created conduit stream", nil)
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

type (
	// errorConduitEvent is returned when the conduit event is malformed
	errorConduitEvent struct{}
)

func (e errorConduitEvent) Error() string {
	return "Malformed conduit event"
}

func decodeConduitEvent(msgdata []byte) (*conduitEvent, error) {
	var m conduitEventDec
	if err := kjson.Unmarshal(msgdata, &m); err != nil {
		return nil, kerrors.WithKind(err, errorConduitEvent{}, "Failed to decode conduit event")
	}
	props := &conduitEvent{
		Kind: m.Kind,
	}
	switch m.Kind {
	case conduitEventKindFriend:
		if err := kjson.Unmarshal(m.Payload, &props.Friend); err != nil {
			return nil, kerrors.WithKind(err, errorConduitEvent{}, "Failed to decode friend event")
		}
	case conduitEventKindUnfriend:
		if err := kjson.Unmarshal(m.Payload, &props.Unfriend); err != nil {
			return nil, kerrors.WithKind(err, errorConduitEvent{}, "Failed to decode unfriend event")
		}
	default:
		return nil, kerrors.WithKind(nil, errorConduitEvent{}, "Invalid user event kind")
	}
	return props, nil
}

func (s *Service) conduitEventHandler(ctx context.Context, msg events.Msg) error {
	props, err := decodeConduitEvent(msg.Value)
	if err != nil {
		return err
	}
	switch props.Kind {
	case conduitEventKindFriend:
		return s.friendEventHandler(ctx, props.Friend)
	case conduitEventKindUnfriend:
		return s.unfriendEventHandler(ctx, props.Unfriend)
	default:
		return nil
	}
}

func (s *Service) userEventHandler(ctx context.Context, props user.UserEvent) error {
	switch props.Kind {
	case user.UserEventKindCreate:
		return s.userCreateEventHandler(ctx, props.Create)
	case user.UserEventKindUpdate:
		return s.userUpdateEventHandler(ctx, props.Update)
	case user.UserEventKindDelete:
		return s.userDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userCreateEventHandler(ctx context.Context, props user.CreateUserProps) error {
	if err := s.friends.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update username")
	}
	return nil
}

func (s *Service) userUpdateEventHandler(ctx context.Context, props user.UpdateUserProps) error {
	if err := s.friends.UpdateUsername(ctx, props.Userid, props.Username); err != nil {
		return kerrors.WithMsg(err, "Failed to update username")
	}
	return nil
}

const (
	chatDeleteBatchSize = 256
)

func (s *Service) userDeleteEventHandler(ctx context.Context, props user.DeleteUserProps) error {
	if err := s.invitations.DeleteByUser(ctx, props.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user invitations")
	}
	for {
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

func (s *Service) friendInvitationGCHook(ctx context.Context, props sysevent.TimestampProps) error {
	if err := s.invitations.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		return kerrors.WithMsg(err, "Failed to GC friend invitations")
	}
	s.log.Info(ctx, "GC friend invitations", nil)
	return nil
}
