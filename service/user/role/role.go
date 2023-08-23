package role

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/role/rolemodel"
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
		roles             rolemodel.Repo
		kvroleset         kvstore.KVStore
		log               *klog.LevelLogger
		roleCacheDuration time.Duration
	}
)

// New returns a new RolesManager
func New(roles rolemodel.Repo, kv kvstore.KVStore, ev events.Events) *Service {
	return &Service{
		roles:     roles,
		kvroleset: kv.Subtree("roleset"),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("rolecacheduration", "24h")
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	var err error
	s.roleCacheDuration, err = r.GetDuration("rolecacheduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse role cache duration")
	}

	s.log.Info(ctx, "Loaded config",
		klog.AString("cacheduration", s.roleCacheDuration.String()),
	)

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
	s.log.Info(ctx, "Created userrole table")

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}
