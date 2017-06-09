package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/token"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	time5m int64 = 300
)

// Conf loads in the default
func Conf(c *governor.Config) error {
	v := c.Conf()
	v.SetDefault("userauth.duration", "5m")
	v.SetDefault("userauth.secret", "governor")
	v.SetDefault("userauth.issuer", "governor")
	return nil
}

const (
	moduleID = "user"
)

type (
	// User is a user management service
	User struct {
		db        *db.Database
		tokenizer *token.Tokenizer
		loginTime int64
	}
)

// New creates a new User
func New(conf governor.Config, db *db.Database) *User {
	c := conf.Conf().GetStringMapString("userauth")
	t := time5m
	if duration, err := time.ParseDuration(c["duration"]); err != nil {
		t = duration.Nanoseconds() / 1000000000
	}
	return &User{
		db:        db,
		tokenizer: token.New(c["secret"], c["issuer"]),
		loginTime: t,
	}
}

const (
	moduleIDUser = moduleID + ".user"
	moduleIDAuth = moduleID + ".auth"
)

// Mount is a collection of routes for accessing and modifying user data
func (u *User) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	if err := u.mountRest(conf, r.Group("/user"), l); err != nil {
		return err
	}
	if err := u.mountAuth(conf, r.Group("/auth"), l); err != nil {
		return err
	}
	return nil
}

// Health is a check for service health
func (u *User) Health() *governor.Error {
	return nil
}
