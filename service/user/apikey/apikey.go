package apikey

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/util/rank"
)

type (
	// Apikey manages apikeys
	Apikey interface {
		GetUserKeys(userid string, limit, offset int) ([]apikeymodel.Model, error)
		CheckKey(keyid, key string) (string, error)
		IntersectRoles(keyid string, authtags rank.Rank) (rank.Rank, error)
		Insert(userid string, authtags rank.Rank, name, desc string) (*ResApikeyModel, error)
		RotateKey(keyid string) (*ResApikeyModel, error)
		UpdateKey(keyid string, authtags rank.Rank, name, desc string) error
		DeleteKey(keyid string) error
		DeleteUserKeys(userid string) error
	}

	Service interface {
		governor.Service
		Apikey
	}

	service struct {
		apikeys       apikeymodel.Repo
		roles         role.Role
		kvkey         kvstore.KVStore
		kvroleset     kvstore.KVStore
		logger        governor.Logger
		roleCacheTime int64
	}
)

const (
	time24h int64 = 86400
	b1            = 1_000_000_000
)

// New returns a new Apikey
func New(apikeys apikeymodel.Repo, roles role.Role, kv kvstore.KVStore) Service {
	return &service{
		apikeys:       apikeys,
		roles:         roles,
		kvkey:         kv.Subtree("key"),
		kvroleset:     kv.Subtree("roleset"),
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

	if err := s.apikeys.Setup(); err != nil {
		return err
	}
	l.Info("created userapikeys table", nil)

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
