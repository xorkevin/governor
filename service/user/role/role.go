package role

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Roles is a read only interface for roles
	Roles interface {
		IntersectRoles(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error)
	}

	// RolesManager manages user roles
	RolesManager interface {
		Roles
		InsertRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteByRole(ctx context.Context, roleName string, userids []string) error
		GetRoles(ctx context.Context, userid string, prefix string, amount, offset int) (rank.Rank, error)
		GetByRole(ctx context.Context, roleName string, amount, offset int) ([]string, error)
	}

	Service struct {
		roles             model.Repo
		kvroleset         kvstore.KVStore
		log               *klog.LevelLogger
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

// GetCtxRolesManager returns a RolesManager service from the context
func GetCtxRolesManager(inj governor.Injector) RolesManager {
	v := inj.Get(ctxKeyRoles{})
	if v == nil {
		return nil
	}
	return v.(RolesManager)
}

// setCtxRolesManager sets a RolesManager service in the context
func setCtxRolesManager(inj governor.Injector, r RolesManager) {
	inj.Set(ctxKeyRoles{}, r)
}

// NewCtx creates a new RolesManager service from a context
func NewCtx(inj governor.Injector) *Service {
	return New(
		model.GetCtxRepo(inj),
		kvstore.GetCtxKVStore(inj),
		events.GetCtxEvents(inj),
	)
}

// New returns a new RolesManager
func New(roles model.Repo, kv kvstore.KVStore, ev events.Events) *Service {
	return &Service{
		roles:     roles,
		kvroleset: kv.Subtree("roleset"),
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxRolesManager(inj, s)

	r.SetDefault("rolecacheduration", "24h")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	var err error
	s.roleCacheDuration, err = r.GetDuration("rolecacheduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse role cache duration")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
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
	if err := s.roles.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userrole table", nil)

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}
