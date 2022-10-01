package role

import (
	"context"
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

type (
	HandlerFunc = func(ctx context.Context, props RolesProps) error

	// Roles manages user roles
	Roles interface {
		IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error)
		InsertRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteByRole(ctx context.Context, roleName string, userids []string) error
		GetRoles(ctx context.Context, userid string, prefix string, amount, offset int) (rank.Rank, error)
		GetByRole(ctx context.Context, roleName string, amount, offset int) ([]string, error)
		WatchRoles(group string, opts events.ConsumerOpts, handler HandlerFunc, dlqhandler HandlerFunc, maxdeliver int) *events.Watcher
	}

	RolesProps struct {
		Add    bool     `json:"add"`
		Userid string   `json:"userid"`
		Roles  []string `json:"roles"`
	}

	Service struct {
		roles             model.Repo
		kvroleset         kvstore.KVStore
		events            events.Events
		instance          string
		log               *klog.LevelLogger
		streamns          string
		streamroles       string
		streamsize        int64
		eventsize         int32
		roleCacheDuration time.Duration
	}

	ctxKeyRoles struct{}
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
		roles:     roles,
		kvroleset: kv.Subtree("roleset"),
		events:    ev,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxRoles(inj, s)
	s.streamns = name
	s.streamroles = name + ".roles"

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("rolecacheduration", "24h")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.instance = c.Instance

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
	s.roleCacheDuration, err = r.GetDuration("rolecacheduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse role cache duration")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"role.stream.size":   r.GetStr("streamsize"),
		"role.event.size":    r.GetStr("eventsize"),
		"role.cacheduration": s.roleCacheDuration.String(),
	})

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.streamroles, events.StreamOpts{
		Partitions:     16,
		Replicas:       1,
		ReplicaQuorum:  1,
		RetentionAge:   30 * 24 * time.Hour,
		RetentionBytes: int(s.streamsize),
		MaxMsgBytes:    int(s.eventsize),
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

func (s *Service) WatchRoles(group string, opts events.ConsumerOpts, handler HandlerFunc, dlqhandler HandlerFunc, maxdeliver int) *events.Watcher {
	var dlqfn events.Handler
	if dlqhandler != nil {
		dlqfn = events.HandlerFunc(func(ctx context.Context, msg events.Msg) error {
			props, err := decodeRolesProps(msg.Value)
			if err != nil {
				return err
			}
			return dlqhandler(ctx, *props)
		})
	}
	return events.NewWatcher(s.events, s.log.Logger, s.streamroles, group, opts, events.HandlerFunc(func(ctx context.Context, msg events.Msg) error {
		props, err := decodeRolesProps(msg.Value)
		if err != nil {
			return err
		}
		return handler(ctx, *props)
	}), dlqfn, maxdeliver, s.instance)
}
