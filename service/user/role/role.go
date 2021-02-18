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

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// Roles manages user roles
	Roles interface {
		IntersectRoles(userid string, roles rank.Rank) (rank.Rank, error)
		InsertRoles(userid string, roles rank.Rank) error
		DeleteRoles(userid string, roles rank.Rank) error
		DeleteAllRoles(userid string) error
		GetRoles(userid string, prefix string, amount, offset int) (rank.Rank, error)
		GetByRole(roleName string, amount, offset int) ([]string, error)
		DeleteByRole(roleName string) error
	}

	// Service is a Roles and governor.Service
	Service interface {
		governor.Service
		Roles
	}

	service struct {
		roles         model.Repo
		kvroleset     kvstore.KVStore
		logger        governor.Logger
		roleCacheTime int64
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
func NewCtx(inj governor.Injector) Service {
	roles := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	return New(roles, kv)
}

// New returns a new Roles
func New(roles model.Repo, kv kvstore.KVStore) Service {
	return &service{
		roles:         roles,
		kvroleset:     kv.Subtree("roleset"),
		roleCacheTime: time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxRoles(inj, s)

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
