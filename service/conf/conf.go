package conf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf/model"
	"github.com/hackform/governor/service/db"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
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
func New(l *logrus.Logger, database db.Database) Conf {
	l.Info("initialized conf service")

	return &confService{
		db: database,
	}
}

// Mount is a collection of routes
func (c *confService) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted conf service")
	return nil
}

// Health is a check for service health
func (c *confService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (c *confService) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if _, err := confmodel.Get(c.db.DB()); err == nil {
		return governor.NewError(moduleID, "setup already run", 128, http.StatusForbidden)
	}

	mconf, err := confmodel.New(rsetup.Orgname)
	if err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new configuration model")

	if err := confmodel.Setup(c.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new configuration table")

	if err := mconf.Insert(c.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("inserted new configuration into config")

	return nil
}
