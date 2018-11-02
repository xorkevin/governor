package conf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf/model"
	"github.com/hackform/governor/service/db"
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
		db db.Database
	}
)

// New creates a new Conf service
func New(l governor.Logger, database db.Database) Conf {
	l.Info("initialized conf service", moduleID, "initialize conf service", 0, nil)

	return &confService{
		db: database,
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
	if _, err := confmodel.Get(c.db.DB()); err == nil {
		return governor.NewError(moduleID, "setup already run", 128, http.StatusForbidden)
	}

	mconf, err := confmodel.New(rsetup.Orgname)
	if err != nil {
		err.AddTrace(moduleID)
		return err
	}

	if err := confmodel.Setup(c.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}

	if err := mconf.Insert(c.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new configuration", moduleID, "create new configuration", 0, nil)

	return nil
}
