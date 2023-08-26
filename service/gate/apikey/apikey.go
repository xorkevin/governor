package apikey

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate/apikey/apikeymodel"
	"xorkevin.dev/klog"
)

type (
	// Apikeys manages apikeys
	Apikeys interface {
		GetUserKeys(ctx context.Context, userid string, limit, offset int) ([]apikeymodel.Model, error)
		CheckKey(ctx context.Context, keyid, key string) (string, string, error)
		Insert(ctx context.Context, userid string, scope string, name, desc string) (*ResApikeyModel, error)
		RotateKey(ctx context.Context, userid string, keyid string) (*ResApikeyModel, error)
		UpdateKey(ctx context.Context, userid string, keyid string, scope string, name, desc string) error
		DeleteKey(ctx context.Context, userid string, keyid string) error
		DeleteKeys(ctx context.Context, keyids []string) error
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
