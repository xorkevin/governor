package apikey

import (
	"context"
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
	// Apikeys manages apikeys
	Apikeys interface {
		GetUserKeys(userid string, limit, offset int) ([]model.Model, error)
		CheckKey(keyid, key string) (string, string, error)
		Insert(userid string, scope string, name, desc string) (*ResApikeyModel, error)
		RotateKey(keyid string) (*ResApikeyModel, error)
		UpdateKey(keyid string, scope string, name, desc string) error
		DeleteKey(keyid string) error
		DeleteUserKeys(userid string) error
	}

	// Service is an Apikeys and governor.Service
	Service interface {
		governor.Service
		Apikeys
	}

	service struct {
		apikeys        model.Repo
		kvkey          kvstore.KVStore
		logger         governor.Logger
		scopeCacheTime int64
	}

	ctxKeyApikeys struct{}
)

// GetCtxApikeys returns a Apikeys service from the context
func GetCtxApikeys(inj governor.Injector) Apikeys {
	v := inj.Get(ctxKeyApikeys{})
	if v == nil {
		return nil
	}
	return v.(Apikeys)
}

// setCtxApikeys sets a Apikeys service in the context
func setCtxApikeys(inj governor.Injector, a Apikeys) {
	inj.Set(ctxKeyApikeys{}, a)
}

// NewCtx returns a new Apikeys service from a context
func NewCtx(inj governor.Injector) Service {
	apikeys := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	return New(apikeys, kv)
}

// New returns a new Apikeys service
func New(apikeys model.Repo, kv kvstore.KVStore) Service {
	return &service{
		apikeys:        apikeys,
		kvkey:          kv.Subtree("key"),
		scopeCacheTime: time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxApikeys(inj, s)

	r.SetDefault("scopecache", "24h")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	if t, err := time.ParseDuration(r.GetStr("scopecache")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse scope cache time")
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

func (s *service) PostSetup(req governor.ReqSetup) error {
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
