package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/mail"
	"github.com/hackform/governor/service/template"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/role/model"
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
	User interface {
		GetByID(userid string) (*ResUserGet, *governor.Error)
	}

	// Service is the public interface for the user service server
	Service interface {
		governor.Service
		User
		RegisterHook(hook Hook)
	}

	userService struct {
		config            governor.Config
		logger            *logrus.Logger
		db                db.Database
		cache             cache.Cache
		tokenizer         *token.Tokenizer
		mailer            mail.Mail
		accessTime        int64
		refreshTime       int64
		confirmTime       int64
		passwordResetTime int64
		newLoginEmail     bool
		passwordMinSize   int
		tpl               template.Template
		gate              gate.Gate
		cc                cachecontrol.CacheControl
		hooks             []Hook
	}

	userRouter struct {
		service userService
	}

	// HookProps are properties of the user passed on to each hook
	HookProps struct {
		Userid    string
		Username  string
		Email     string
		FirstName string
		LastName  string
	}

	// Hook is a service that can hook onto the user create and destroy pipelines
	Hook interface {
		UserCreateHook(props HookProps) *governor.Error
		UserDeleteHook(userid string) *governor.Error
	}
)

const (
	time15m int64 = 900
	time7d  int64 = 604800
	time24h int64 = 86400
	b1            = 1000000000
)

// New creates a new User
func New(conf governor.Config, l *logrus.Logger, database db.Database, ch cache.Cache, m mail.Mail, tpl template.Template, g gate.Gate, cc cachecontrol.CacheControl) Service {
	c := conf.Conf()
	ca := c.GetStringMapString("userauth")
	cu := c.GetStringMapString("user")
	accessTime := time15m
	refreshTime := time7d
	confirmTime := time24h
	passwordResetTime := time24h
	if duration, err := time.ParseDuration(ca["duration"]); err == nil {
		accessTime = duration.Nanoseconds() / b1
		l.Infof("auth: duration: %ds", accessTime)
	} else {
		l.Warnf("auth: failed to parse duration: %s", ca["duration"])
	}
	if duration, err := time.ParseDuration(ca["refresh_duration"]); err == nil {
		refreshTime = duration.Nanoseconds() / b1
		l.Infof("auth: refresh_duration: %ds", refreshTime)
	} else {
		l.Warnf("auth: failed to parse refresh_duration: %s", ca["refresh_duration"])
	}
	if duration, err := time.ParseDuration(cu["confirm_duration"]); err == nil {
		confirmTime = duration.Nanoseconds() / b1
		l.Infof("auth: confirm_duration: %ds", confirmTime)
	} else {
		l.Warnf("auth: failed to parse confirm_duration: %s", ca["confirm_duration"])
	}
	if duration, err := time.ParseDuration(cu["password_reset_duration"]); err == nil {
		passwordResetTime = duration.Nanoseconds() / b1
		l.Infof("auth: password_reset_duration: %ds", passwordResetTime)
	} else {
		l.Warnf("auth: failed to parse password_reset_duration: %s", ca["password_reset_duration"])
	}
	l.Infof("auth: issuer: %s", ca["issuer"])

	l.Infof("user: new_login_email: %t", c.GetBool("user.new_login_email"))
	l.Infof("user: password_min_size: %d", c.GetInt("user.password_min_size"))
	l.Info("initialized user service")

	return &userService{
		config:            conf,
		logger:            l,
		db:                database,
		cache:             ch,
		mailer:            m,
		tokenizer:         token.New(ca["secret"], ca["issuer"]),
		accessTime:        accessTime,
		refreshTime:       refreshTime,
		confirmTime:       confirmTime,
		passwordResetTime: passwordResetTime,
		newLoginEmail:     c.GetBool("user.new_login_email"),
		passwordMinSize:   c.GetInt("user.password_min_size"),
		tpl:               tpl,
		gate:              g,
		cc:                cc,
		hooks:             []Hook{},
	}
}

func (u *userService) newRouter() *userRouter {
	return &userRouter{
		service: *u,
	}
}

const (
	moduleIDUser = moduleID + ".user"
	moduleIDAuth = moduleID + ".auth"
)

// Mount is a collection of routes for accessing and modifying user data
func (u *userService) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	ur := u.newRouter()
	if err := ur.mountRoute(conf, r.Group("/user")); err != nil {
		return err
	}
	if err := ur.mountAuth(conf, r.Group("/auth")); err != nil {
		return err
	}
	l.Info("mounted user service")
	return nil
}

// Health is a check for service health
func (u *userService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (u *userService) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
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

	if err := rolemodel.Setup(u.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new userrole table")

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

// RegisterHook adds a hook to the user create and destroy pipelines
func (u *userService) RegisterHook(hook Hook) {
	u.hooks = append(u.hooks, hook)
}
