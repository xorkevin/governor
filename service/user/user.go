package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/mail"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/model"
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
		db                db.Database
		cache             cache.Cache
		tokenizer         *token.Tokenizer
		mailer            mail.Mail
		accessTime        int64
		refreshTime       int64
		confirmTime       int64
		passwordResetTime int64
		gate              gate.Gate
	}
)

const (
	time15m int64 = 900
	time7d  int64 = 604800
	time24h int64 = 86400
	b1            = 1000000000
)

// New creates a new User
func New(conf governor.Config, l *logrus.Logger, database db.Database, ch cache.Cache, m mail.Mail) *User {
	ca := conf.Conf().GetStringMapString("userauth")
	cu := conf.Conf().GetStringMapString("user")
	accessTime := time15m
	refreshTime := time7d
	confirmTime := time24h
	passwordResetTime := time24h
	if duration, err := time.ParseDuration(ca["duration"]); err == nil {
		accessTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(ca["refresh_duration"]); err == nil {
		refreshTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(cu["confirm_duration"]); err == nil {
		confirmTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(cu["password_reset_duration"]); err == nil {
		passwordResetTime = duration.Nanoseconds() / b1
	}

	l.Info("initialized user service")

	return &User{
		db:                database,
		cache:             ch,
		mailer:            m,
		tokenizer:         token.New(ca["secret"], ca["issuer"]),
		accessTime:        accessTime,
		refreshTime:       refreshTime,
		confirmTime:       confirmTime,
		passwordResetTime: passwordResetTime,
		gate:              gate.New(ca["secret"], ca["issuer"]),
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

	l.Info("mounted user service")

	return nil
}

// Health is a check for service health
func (u *User) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (u *User) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	madmin, err := usermodel.NewAdmin(rsetup.Username, rsetup.Password, rsetup.Email, rsetup.Firstname, rsetup.Lastname)
	if err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new admin model")

	if err := usermodel.Setup(u.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new user table")

	if err := madmin.Insert(u.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	userid, _ := madmin.IDBase64()
	l.WithFields(logrus.Fields{
		"username": madmin.Username,
		"userid":   userid,
	}).Info("inserted new admin into users")

	return nil
}
