package conf

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf/model"
	"github.com/hackform/governor/service/db"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

const (
	moduleID = "conf"
)

type (
	// Conf is a configuration service for admins
	Conf struct {
		db db.Database
	}
)

// New creates a new Conf service
func New(l *logrus.Logger, database db.Database) *Conf {
	l.Info("initialized conf service")

	return &Conf{
		db: database,
	}
}

// Mount is a collection of routes
func (c *Conf) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	l.Info("mounted conf service")
	return nil
}

// Health is a check for service health
func (c *Conf) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (c *Conf) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
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
