package role

import (
	"context"
	"strconv"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// WorkerFunc is a type alias for a role event consumer
	WorkerFunc = func(ctx context.Context, pinger events.Pinger, props RolesProps) error

	// Roles manages user roles
	Roles interface {
		IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error)
		InsertRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteByRole(ctx context.Context, roleName string, userids []string) error
		GetRoles(ctx context.Context, userid string, prefix string, amount, offset int) (rank.Rank, error)
		GetByRole(ctx context.Context, roleName string, amount, offset int) ([]string, error)
		StreamSubscribeCreate(group string, worker WorkerFunc, streamopts events.StreamConsumerOpts) (events.Subscription, error)
		StreamSubscribeDelete(group string, worker WorkerFunc, streamopts events.StreamConsumerOpts) (events.Subscription, error)
	}

	RolesProps struct {
		Userid string
		Roles  []string
	}

	Service struct {
		roles         model.Repo
		kvroleset     kvstore.KVStore
		events        events.Events
		log           *klog.LevelLogger
		streamns      string
		opts          svcOpts
		streamsize    int64
		eventsize     int32
		roleCacheTime int64
	}

	ctxKeyRoles struct{}

	svcOpts struct {
		StreamName    string
		CreateChannel string
		DeleteChannel string
	}
)

// GetCtxRoles returns a Roles service from the context
func GetCtxRoles(inj governor.Injector) Roles {
	v := inj.Get(ctxKeyRoles{})
	if v == nil {
		return nil
	}
	return v.(Roles)
}

// setCtxRoles sets a Roles service in the context
func setCtxRoles(inj governor.Injector, r Roles) {
	inj.Set(ctxKeyRoles{}, r)
}

// NewCtx creates a new Roles service from a context
func NewCtx(inj governor.Injector) *Service {
	roles := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	ev := events.GetCtxEvents(inj)
	return New(roles, kv, ev)
}

// New returns a new Roles
func New(roles model.Repo, kv kvstore.KVStore, ev events.Events) *Service {
	return &Service{
		roles:         roles,
		kvroleset:     kv.Subtree("roleset"),
		events:        ev,
		roleCacheTime: time24h,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxRoles(inj, s)
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = svcOpts{
		StreamName:    streamname,
		CreateChannel: streamname + ".create",
		DeleteChannel: streamname + ".delete",
	}

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("rolecache", "24h")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)
	if t, err := time.ParseDuration(r.GetStr("rolecache")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse role cache time")
	} else {
		s.roleCacheTime = int64(t / time.Second)
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"role.stream.size": r.GetStr("streamsize"),
		"role.event.size":  r.GetStr("eventsize"),
		"role.cache":       strconv.FormatInt(s.roleCacheTime, 10),
	})

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init roles stream")
	}
	s.log.Info(ctx, "Created roles stream", nil)

	if err := s.roles.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userrole table", nil)

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func decodeRolesProps(msgdata []byte) (*RolesProps, error) {
	m := &RolesProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode roles props")
	}
	return m, nil
}

func (s *Service) StreamSubscribeCreate(group string, worker WorkerFunc, streamopts events.StreamConsumerOpts) (events.Subscription, error) {
	sub, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.CreateChannel, group, func(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
		props, err := decodeRolesProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, pinger, *props)
	}, streamopts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to role create channel")
	}
	return sub, nil
}

func (s *Service) StreamSubscribeDelete(group string, worker WorkerFunc, streamopts events.StreamConsumerOpts) (events.Subscription, error) {
	sub, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DeleteChannel, group, func(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
		props, err := decodeRolesProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, pinger, *props)
	}, streamopts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to role delete channel")
	}
	return sub, nil
}
