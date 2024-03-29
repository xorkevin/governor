package apikey

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate/apikey/apikeymodel"
	"xorkevin.dev/klog"
)

type (
	UserScope struct {
		Userid string
		Scope  string
	}

	// Validator checks apikeys
	Validator interface {
		Check(ctx context.Context, keyid, key string) (*UserScope, error)
	}

	// Apikeys manages apikeys
	Apikeys interface {
		Validator
		GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]Props, error)
		InsertKey(ctx context.Context, userid string, scope string, name, desc string) (*Key, error)
		RotateKey(ctx context.Context, userid string, keyid string) (*Key, error)
		UpdateKey(ctx context.Context, userid string, keyid string, scope string, name, desc string) error
		DeleteKey(ctx context.Context, userid string, keyid string) error
		DeleteUserKeys(ctx context.Context, userid string) error
	}

	Key struct {
		Keyid string `json:"keyid"`
		Key   string `json:"key"`
	}

	Props struct {
		Keyid        string `json:"keyid"`
		Userid       string `json:"userid"`
		Scope        string `json:"scope"`
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		RotateTime   int64  `json:"rotate_time"`
		UpdateTime   int64  `json:"update_time"`
		CreationTime int64  `json:"creation_time"`
	}

	Service struct {
		apikeys apikeymodel.Repo
		log     *klog.LevelLogger
	}
)

// New returns a new Apikeys service
func New(apikeys apikeymodel.Repo) *Service {
	return &Service{
		apikeys: apikeys,
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
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
	s.log.Info(ctx, "Created apikeys table")

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}
