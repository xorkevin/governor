package user

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/mail"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/role/model"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/rank"
	"github.com/labstack/echo"
	"strconv"
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
		logger            governor.Logger
		repo              usermodel.Repo
		rolerepo          rolemodel.Repo
		cache             cache.Cache
		tokenizer         *token.Tokenizer
		mailer            mail.Mail
		accessTime        int64
		refreshTime       int64
		confirmTime       int64
		passwordResetTime int64
		newLoginEmail     bool
		passwordMinSize   int
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
func New(conf governor.Config, l governor.Logger, repo usermodel.Repo, rolerepo rolemodel.Repo, ch cache.Cache, m mail.Mail, g gate.Gate, cc cachecontrol.CacheControl) Service {
	c := conf.Conf()
	ca := c.GetStringMapString("userauth")
	cu := c.GetStringMapString("user")
	accessTime := time15m
	refreshTime := time7d
	confirmTime := time24h
	passwordResetTime := time24h
	if duration, err := time.ParseDuration(ca["duration"]); err == nil {
		accessTime = duration.Nanoseconds() / b1
	} else {
		l.Warn(fmt.Sprintf("auth: failed to parse duration: %s", ca["duration"]), moduleID, "fail parse access duration", 0, nil)
	}
	if duration, err := time.ParseDuration(ca["refresh_duration"]); err == nil {
		refreshTime = duration.Nanoseconds() / b1
	} else {
		l.Warn(fmt.Sprintf("auth: failed to parse refresh_duration: %s", ca["refresh_duration"]), moduleID, "fail parse refresh duration", 0, nil)
	}
	if duration, err := time.ParseDuration(cu["confirm_duration"]); err == nil {
		confirmTime = duration.Nanoseconds() / b1
	} else {
		l.Warn(fmt.Sprintf("auth: failed to parse confirm_duration: %s", ca["confirm_duration"]), moduleID, "fail parse confirm duration", 0, nil)
	}
	if duration, err := time.ParseDuration(cu["password_reset_duration"]); err == nil {
		passwordResetTime = duration.Nanoseconds() / b1
	} else {
		l.Warn(fmt.Sprintf("auth: failed to parse password_reset_duration: %s", ca["password_reset_duration"]), moduleID, "fail parse password reset duration", 0, nil)
	}
	l.Info("initialized user service", moduleID, "initialize user service", 0, map[string]string{
		"auth: duration (s)":                strconv.FormatInt(accessTime, 10),
		"auth: refresh_duration (s)":        strconv.FormatInt(refreshTime, 10),
		"auth: confirm_duration (s)":        strconv.FormatInt(confirmTime, 10),
		"auth: password_reset_duration (s)": strconv.FormatInt(passwordResetTime, 10),
		"auth: issuer":                      ca["issuer"],
		"user: new_login_email":             strconv.FormatBool(c.GetBool("user.new_login_email")),
		"user: password_min_size":           strconv.Itoa(c.GetInt("user.password_min_size")),
	})

	return &userService{
		config:            conf,
		logger:            l,
		repo:              repo,
		rolerepo:          rolerepo,
		cache:             ch,
		mailer:            m,
		tokenizer:         token.New(ca["secret"], ca["issuer"]),
		accessTime:        accessTime,
		refreshTime:       refreshTime,
		confirmTime:       confirmTime,
		passwordResetTime: passwordResetTime,
		newLoginEmail:     c.GetBool("user.new_login_email"),
		passwordMinSize:   c.GetInt("user.password_min_size"),
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
func (u *userService) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	ur := u.newRouter()
	if err := ur.mountRoute(conf, r.Group("/user")); err != nil {
		return err
	}
	if err := ur.mountAuth(conf, r.Group("/auth")); err != nil {
		return err
	}
	l.Info("mounted user service", moduleID, "mount user service", 0, nil)
	return nil
}

// Health is a check for service health
func (u *userService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (u *userService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	madmin, err := u.repo.New(rsetup.Username, rsetup.Password, rsetup.Email, rsetup.Firstname, rsetup.Lastname, rank.Admin())
	if err != nil {
		err.AddTrace(moduleID)
		return err
	}

	if err := u.repo.Setup(); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new user table", moduleID, "create user table", 0, nil)

	if err := u.rolerepo.Setup(); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new userrole table", moduleID, "create userrole table", 0, nil)

	if err := u.repo.Insert(madmin); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	userid, _ := madmin.IDBase64()
	l.Info("inserted new admin into users", moduleID, "insert new setup admin", 0, map[string]string{
		"username": madmin.Username,
		"userid":   userid,
	})

	return nil
}

// RegisterHook adds a hook to the user create and destroy pipelines
func (u *userService) RegisterHook(hook Hook) {
	u.hooks = append(u.hooks, hook)
}
