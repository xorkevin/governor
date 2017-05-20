package user

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

const (
	moduleID = "user"
)

type (
	// User is a user management service
	User struct {
	}
)

// New creates a new User
func New() *User {
	return &User{}
}

const (
	moduleIDUser = moduleID + ".user"
)

// Mount is a collection of routes for accessing and modifying user data
func (u *User) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	if err := mountRest(conf, r.Group("/user"), db, l); err != nil {
		return err
	}
	if err := mountAuth(conf, r.Group("/auth"), db, l); err != nil {
		return err
	}
	return nil
}

// Health is a check for service health
func (u *User) Health() *governor.Error {
	return nil
}
