package role

import (
	"context"
	"net/http"
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
		GetRoles(userid string, prefix string, amount, offset int) (rank.Rank, error)
		GetByRole(roleName string, amount, offset int) ([]string, error)
		DeleteByRole(roleName string) error
	}

	Service interface {
		governor.Service
		Role
	}

	service struct {
		roles         rolemodel.Repo
		kvroleset     kvstore.KVStore
		logger        governor.Logger
		roleCacheTime int64
	}
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

// New returns a new Role
func New(roles rolemodel.Repo, kv kvstore.KVStore) Service {
	return &service{
		roles:         roles,
		kvroleset:     kv.Subtree("roleset"),
		roleCacheTime: time24h,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("rolecache", "24h")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	if t, err := time.ParseDuration(r.GetStr("rolecache")); err != nil {
		return governor.NewError("Failed to parse role cache time", http.StatusBadRequest, err)
	} else {
		s.roleCacheTime = int64(t / time.Second)
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
