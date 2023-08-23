package conduit

import (
	"context"
	"encoding/json"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit/dmmodel"
	"xorkevin.dev/governor/service/conduit/friendinvmodel"
	"xorkevin.dev/governor/service/conduit/friendmodel"
	"xorkevin.dev/governor/service/conduit/gdmmodel"
	"xorkevin.dev/governor/service/conduit/msgmodel"
	"xorkevin.dev/governor/service/conduit/servermodel"
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
	Conduit interface{}

	Service struct {
		friends            friendmodel.Repo
		invitations        friendinvmodel.Repo
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
		config             governor.ConfigReader
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
		wg                 *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}

	svcOpts struct {
		PresenceQueryChannel string
		DMMsgChannel         string
		DMSettingsChannel    string
		GDMMsgChannel        string
		GDMSettingsChannel   string
	}
)

// New creates a new Conduit service
func New(
	friends friendmodel.Repo,
	invitations friendinvmodel.Repo,
	dms dmmodel.Repo,
	gdms gdmmodel.Repo,
	servers servermodel.Repo,
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
		servers:     servers,
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

func (s *Service) Register(r governor.ConfigRegistrar) {
	s.scopens = "gov." + r.Name()
	s.channelns = r.Name()
	s.streamns = r.Name()
	s.streamconduit = r.Name()
	s.opts = svcOpts{
		PresenceQueryChannel: s.channelns + ".presence",
		DMMsgChannel:         s.channelns + ".chat.dm.msg",
		DMSettingsChannel:    s.channelns + ".chat.dm.settings",
		GDMMsgChannel:        s.channelns + ".chat.gdm.msg",
		GDMSettingsChannel:   s.channelns + ".chat.gdm.settings",
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

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r

	var err error
	s.invitationDuration, err = r.GetDuration("invitationduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse friend invitation duration")
	}
	s.gcDuration, err = r.GetDuration("gcduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse gc duration")
	}

	s.log.Info(ctx, "Loaded config",
		klog.AString("streamsize", r.GetStr("streamsize")),
		klog.AString("eventsize", r.GetStr("eventsize")),
		klog.AString("invitationduration", s.invitationDuration.String()),
	)

	sr := s.router()
	sr.mountRoutes(m)
	s.log.Info(ctx, "Mounted http routes")
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
		s.config.Config().Instance,
	).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to conduit stream")

	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream")

	sysEvents := sysevent.New(s.config.Config(), s.pubsub, s.log.Logger)
	s.wg.Add(1)
	go sysEvents.WatchGC(s.streamns+"_WORKER_INVITATION_GC", s.friendInvitationGCHook, s.config.Config().Instance).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to gov sys gc channel")

	s.wg.Add(1)
	go s.ws.WatchPresence(s.channelns+".>", s.streamns+"_WORKER_PRESENCE", s.presenceHandler).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to ws presence channel")

	s.wg.Add(1)
	go s.ws.Watch(s.opts.PresenceQueryChannel, s.streamns+"_PRESENCE_QUERY", s.presenceQueryHandler).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to ws conduit presence query channel")

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.friends.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit friend table")
	if err := s.invitations.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit friend invitation table")
	if err := s.dms.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit dm table")
	if err := s.gdms.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit gdm tables")
	if err := s.msgs.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created conduit msg table")
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
	s.log.Info(ctx, "Created conduit stream")
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

type (
	// errConduitEvent is returned when the conduit event is malformed
	errConduitEvent struct{}
)

func (e errConduitEvent) Error() string {
	return "Malformed conduit event"
}

func decodeConduitEvent(msgdata []byte) (*conduitEvent, error) {
	var m conduitEventDec
	if err := kjson.Unmarshal(msgdata, &m); err != nil {
		return nil, kerrors.WithKind(err, errConduitEvent{}, "Failed to decode conduit event")
	}
	props := &conduitEvent{
		Kind: m.Kind,
	}
	switch m.Kind {
	case conduitEventKindFriend:
		if err := kjson.Unmarshal(m.Payload, &props.Friend); err != nil {
			return nil, kerrors.WithKind(err, errConduitEvent{}, "Failed to decode friend event")
		}
	case conduitEventKindUnfriend:
		if err := kjson.Unmarshal(m.Payload, &props.Unfriend); err != nil {
			return nil, kerrors.WithKind(err, errConduitEvent{}, "Failed to decode unfriend event")
		}
	default:
		return nil, kerrors.WithKind(nil, errConduitEvent{}, "Invalid user event kind")
	}
	return props, nil
}

func encodeConduitEventFriend(props friendProps) ([]byte, error) {
	b, err := kjson.Marshal(conduitEventEnc{
		Kind:    conduitEventKindFriend,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode friend props to json")
	}
	return b, nil
}

func encodeConduitEventUnfriend(props unfriendProps) ([]byte, error) {
	b, err := kjson.Marshal(conduitEventEnc{
		Kind:    conduitEventKindUnfriend,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode unfriend props to json")
	}
	return b, nil
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
	s.log.Info(ctx, "GC friend invitations")
	return nil
}
