package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/token"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	moduleID = "user"
)

type (
	// User is a user management service
	User struct {
		db          *db.Database
		cache       *cache.Cache
		tokenizer   *token.Tokenizer
		accessTime  int64
		refreshTime int64
		gate        *gate.Gate
	}
)

const (
	time15m int64 = 900
	time7d  int64 = 604800
	b1            = 1000000000
)

// New creates a new User
func New(conf governor.Config, db *db.Database, ch *cache.Cache) *User {
	c := conf.Conf().GetStringMapString("userauth")
	atime := time15m
	rtime := time7d
	if duration, err := time.ParseDuration(c["duration"]); err != nil {
		atime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(c["refresh_duration"]); err != nil {
		rtime = duration.Nanoseconds() / b1
	}
	return &User{
		db:          db,
		cache:       ch,
		tokenizer:   token.New(c["secret"], c["issuer"]),
		accessTime:  atime,
		refreshTime: rtime,
		gate:        gate.New(c["secret"], c["issuer"]),
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
