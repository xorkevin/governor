package apikey

import (
	"context"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// Apikeys manages apikeys
	Apikeys interface {
		GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]model.Model, error)
		CheckKey(ctx context.Context, keyid, key string) (string, string, error)
		Insert(ctx context.Context, userid string, scope string, name, desc string) (*ResApikeyModel, error)
		RotateKey(ctx context.Context, keyid string) (*ResApikeyModel, error)
		UpdateKey(ctx context.Context, keyid string, scope string, name, desc string) error
		DeleteKey(ctx context.Context, keyid string) error
		DeleteKeys(ctx context.Context, keyids []string) error
	}

	Service struct {
		apikeys            model.Repo
		kvkey              kvstore.KVStore
		log                *klog.LevelLogger
		scopeCacheDuration time.Duration
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
func NewCtx(inj governor.Injector) *Service {
	apikeys := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	return New(apikeys, kv)
}

// New returns a new Apikeys service
func New(apikeys model.Repo, kv kvstore.KVStore) *Service {
	return &Service{
		apikeys: apikeys,
		kvkey:   kv.Subtree("key"),
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxApikeys(inj, s)

	r.SetDefault("scopecache", "24h")
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	var err error
	s.scopeCacheDuration, err = r.GetDuration("scopecache")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse scope cache time")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"apikey.scopecache": s.scopeCacheDuration.String(),
	})

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.apikeys.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userapikeys table", nil)

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}
