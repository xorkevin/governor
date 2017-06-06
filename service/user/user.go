package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

const (
	moduleID = "user"
)

type (
	// User is a user management service
	User struct {
		db *db.Database
	}
)

// New creates a new User
func New(db *db.Database) *User {
	return &User{
		db: db,
	}
}

const (
	moduleIDUser = moduleID + ".user"
)

// Mount is a collection of routes for accessing and modifying user data
func (u *User) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	sdb := u.db.DB()
	if err := mountRest(conf, r.Group("/user"), sdb, l); err != nil {
		return err
	}
	if err := mountAuth(conf, r.Group("/auth"), sdb, l); err != nil {
		return err
	}
	return nil
}

// Health is a check for service health
func (u *User) Health() *governor.Error {
	return nil
}
