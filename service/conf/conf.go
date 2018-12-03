package conf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf/model"
	"github.com/labstack/echo"
	"net/http"
)

const (
	moduleID = "conf"
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
	l.Info("initialized conf service", moduleID, "initialize conf service", 0, nil)

	return &confService{
		repo: repo,
	}
}

// Mount is a collection of routes
func (c *confService) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	l.Info("mounted conf service", moduleID, "mont conf service", 0, nil)
	return nil
}

// Health is a check for service health
func (c *confService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (c *confService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if _, err := c.repo.Get(); err == nil {
		return governor.NewError(moduleID, "setup already run", 128, http.StatusForbidden)
	}

	mconf, err := c.repo.New(rsetup.Orgname)
	if err != nil {
		err.AddTrace(moduleID)
		return err
	}

	if err := c.repo.Setup(); err != nil {
		err.AddTrace(moduleID)
		return err
	}

	if err := c.repo.Insert(mconf); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new configuration", moduleID, "create new configuration", 0, nil)

	return nil
}
