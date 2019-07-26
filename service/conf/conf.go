package conf

import (
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conf/model"
)

type (
	// Conf is a configuration service for admins
	Conf interface {
		governor.Service
	}

	confService struct {
		repo confmodel.Repo
	}
)

// New creates a new Conf service
func New(conf governor.Config, l governor.Logger, repo confmodel.Repo) Conf {
	l.Info("initialize conf service", nil)

	return &confService{
		repo: repo,
	}
}

// Mount is a collection of routes
func (c *confService) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mount conf service", nil)
	return nil
}

// Health is a check for service health
func (c *confService) Health() error {
	return nil
}

// Setup is run on service setup
func (c *confService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	if _, err := c.repo.Get(); err == nil {
		return governor.NewError("Setup already run", http.StatusForbidden, nil)
	} else if governor.ErrorStatus(err) != http.StatusNotFound {
		return err
	}

	mconf := c.repo.New(rsetup.Orgname)

	if err := c.repo.Setup(); err != nil {
		return err
	}

	if err := c.repo.Insert(mconf); err != nil {
		return err
	}

	l.Info("create new configuration", nil)
	return nil
}
