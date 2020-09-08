package apikey

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/apikey/model"
)

type (
	// Apikey manages apikeys
	Apikey interface {
		GetUserKeys(userid string, limit, offset int) ([]apikeymodel.Model, error)
		CheckKey(keyid, key string) (string, string, error)
		Insert(userid string, scope string, name, desc string) (*ResApikeyModel, error)
		RotateKey(keyid string) (*ResApikeyModel, error)
		UpdateKey(keyid string, scope string, name, desc string) error
		DeleteKey(keyid string) error
		DeleteUserKeys(userid string) error
	}

	Service interface {
		governor.Service
		Apikey
	}

	service struct {
		apikeys        apikeymodel.Repo
		kvkey          kvstore.KVStore
		kvscope        kvstore.KVStore
		logger         governor.Logger
		scopeCacheTime int64
	}
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

// New returns a new Apikey
func New(apikeys apikeymodel.Repo, kv kvstore.KVStore) Service {
	return &service{
		apikeys:        apikeys,
		kvkey:          kv.Subtree("key"),
		kvscope:        kv.Subtree("scope"),
		scopeCacheTime: time24h,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("scopecache", "24h")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	if t, err := time.ParseDuration(r.GetStr("scopecache")); err != nil {
		return governor.NewError("Failed to parse scope cache time", http.StatusBadRequest, err)
	} else {
		s.scopeCacheTime = int64(t / time.Second)
	}

	l.Info("loaded config", map[string]string{
		"scopecache (s)": strconv.FormatInt(s.scopeCacheTime, 10),
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
