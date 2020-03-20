package apikey

import (
	"context"
	"github.com/labstack/echo/v4"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/util/rank"
)

type (
	// Apikey manages apikeys
	Apikey interface {
		GetUserKeys(userid string, limit, offset int) ([]apikeymodel.Model, error)
		CheckKey(userid, keyid, key string, authtags rank.Rank) error
		Insert(userid string, authtags rank.Rank, name, desc string) (*ResApikeyModel, error)
		RotateKey(keyid string) (*ResApikeyModel, error)
		UpdateKey(userid, keyid string, authtags rank.Rank, name, desc string) error
		DeleteKey(keyid string) error
		DeleteUserKeys(userid string) error
	}

	Service interface {
		governor.Service
		Apikey
	}

	service struct {
		apikeys apikeymodel.Repo
		roles   role.Role
		logger  governor.Logger
	}
)

const (
	time24h int64 = 86400
	b1            = 1_000_000_000
)

// New returns a new Apikey
func New(apikeys apikeymodel.Repo, roles role.Role) Service {
	return &service{
		apikeys: apikeys,
		roles:   roles,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
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
