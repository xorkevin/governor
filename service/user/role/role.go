package role

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/util/rank"
)

type (
	// Role manages user roles
	Role interface {
		IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error)
		InsertRoles(userid string, roles rank.Rank) error
		DeleteRoles(userid string, roles rank.Rank) error
		DeleteAllRoles(userid string) error
		GetRoles(userid string, amount, offset int) (rank.Rank, error)
		GetByRole(roleName string, amount, offset int) ([]string, error)
		GetRoleSummary(userid string) (rank.Rank, error)
	}

	Service interface {
		governor.Service
		Role
	}

	service struct {
		roles         rolemodel.Repo
		kvroleset     kvstore.KVStore
		kvsummary     kvstore.KVStore
		logger        governor.Logger
		roleCacheTime int64
	}
)

const (
	time24h int64 = 86400
	b1            = 1_000_000_000
)

// New returns a new Role
func New(roles rolemodel.Repo, kv kvstore.KVStore) Service {
	return &service{
		roles:         roles,
		kvroleset:     kv.Subtree("roleset"),
		kvsummary:     kv.Subtree("summary"),
		roleCacheTime: time24h,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("rolecache", "24h")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	if t, err := time.ParseDuration(r.GetStr("rolecache")); err != nil {
		l.Warn(fmt.Sprintf("failed to parse role cache time: %s", r.GetStr("rolecache")), nil)
	} else {
		s.roleCacheTime = t.Nanoseconds() / b1
	}

	l.Info("loaded config", map[string]string{
		"rolecache (s)": strconv.FormatInt(s.roleCacheTime, 10),
	})

	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.roles.Setup(); err != nil {
		return err
	}
	l.Info("created userrole table", nil)

	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
