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

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
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
		logger         governor.Logger
		scopeCacheTime int64
	}

	ctxKeyApikey struct{}
)

// GetCtxApikey returns a Apikey service from the context
func GetCtxApikey(inj governor.Injector) Apikey {
	v := inj.Get(ctxKeyApikey{})
	if v == nil {
		return nil
	}
	return v.(Apikey)
}

// setCtxApikey sets a Apikey service in the context
func setCtxApikey(inj governor.Injector, a Apikey) {
	inj.Set(ctxKeyApikey{}, a)
}

// NewCtx returns a new Apikey service from a context
func NewCtx(inj governor.Injector) Service {
	apikeys := apikeymodel.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	return New(apikeys, kv)
}

// New returns a new Apikey service
func New(apikeys apikeymodel.Repo, kv kvstore.KVStore) Service {
	return &service{
		apikeys:        apikeys,
		kvkey:          kv.Subtree("key"),
		scopeCacheTime: time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxApikey(inj, s)

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
