package user

import (
	"fmt"
	"github.com/labstack/echo"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cache"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

const (
	moduleID = "user"
)

type (
	// User is a user management service
	User interface {
		GetByID(userid string) (*ResUserGet, error)
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
		sessionrepo       sessionmodel.Repo
		cache             cache.Cache
		tokenizer         *token.Tokenizer
		mailer            mail.Mail
		accessTime        int64
		refreshTime       int64
		refreshCacheTime  int64
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
		UserCreateHook(props HookProps) error
		UserDeleteHook(userid string) error
	}
)

const (
	time15m    int64 = 900
	time24h    int64 = 86400
	time6month int64 = time24h * 365 / 2
	b1               = 1000000000
)

// New creates a new User
func New(conf governor.Config, l governor.Logger, repo usermodel.Repo, rolerepo rolemodel.Repo, sessionrepo sessionmodel.Repo, ch cache.Cache, m mail.Mail, g gate.Gate, cc cachecontrol.CacheControl) Service {
	c := conf.Conf()
	ca := c.GetStringMapString("userauth")
	cu := c.GetStringMapString("user")
	accessTime := time15m
	refreshTime := time6month
	refreshCacheTime := time24h
	confirmTime := time24h
	passwordResetTime := time24h
	if duration, err := time.ParseDuration(ca["duration"]); err != nil {
		l.Warn(fmt.Sprintf("auth: fail to parse access duration: %s", ca["duration"]), nil)
	} else {
		accessTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(ca["refresh_duration"]); err != nil {
		l.Warn(fmt.Sprintf("auth: fail to parse refresh_duration: %s", ca["refresh_duration"]), nil)
	} else {
		refreshTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(ca["refresh_cache_duration"]); err != nil {
		l.Warn(fmt.Sprintf("auth: fail to parse refresh_cache_duration: %s", ca["refresh_cache_duration"]), nil)
	} else {
		refreshCacheTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(cu["confirm_duration"]); err != nil {
		l.Warn(fmt.Sprintf("auth: fail to parse confirm_duration: %s", ca["confirm_duration"]), nil)
	} else {
		confirmTime = duration.Nanoseconds() / b1
	}
	if duration, err := time.ParseDuration(cu["password_reset_duration"]); err != nil {
		l.Warn(fmt.Sprintf("auth: fail to parse password_reset_duration: %s", ca["password_reset_duration"]), nil)
	} else {
		passwordResetTime = duration.Nanoseconds() / b1
	}
	l.Info("initialize user service", map[string]string{
		"auth: duration (s)":                strconv.FormatInt(accessTime, 10),
		"auth: refresh_duration (s)":        strconv.FormatInt(refreshTime, 10),
		"auth: refresh_cache_duration (s)":  strconv.FormatInt(refreshCacheTime, 10),
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
		sessionrepo:       sessionrepo,
		cache:             ch,
		mailer:            m,
		tokenizer:         token.New(ca["secret"], ca["issuer"]),
		accessTime:        accessTime,
		refreshTime:       refreshTime,
		refreshCacheTime:  refreshCacheTime,
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

// Mount is a collection of routes for accessing and modifying user data
func (u *userService) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	ur := u.newRouter()
	if err := ur.mountRoute(conf, r.Group("/user")); err != nil {
		return err
	}
	if err := ur.mountAuth(conf, r.Group("/auth")); err != nil {
		return err
	}
	l.Info("mount user service", nil)
	return nil
}

// Health is a check for service health
func (u *userService) Health() error {
	return nil
}

// Setup is run on service setup
func (u *userService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	madmin, err := u.repo.New(rsetup.Username, rsetup.Password, rsetup.Email, rsetup.Firstname, rsetup.Lastname, rank.Admin())
	if err != nil {
		return err
	}

	if err := u.repo.Setup(); err != nil {
		return err
	}
	l.Info("create user table", nil)

	if err := u.rolerepo.Setup(); err != nil {
		return err
	}
	l.Info("create userrole table", nil)

	if err := u.repo.Insert(madmin); err != nil {
		return err
	}
	l.Info("insert new setup admin", map[string]string{
		"username": madmin.Username,
		"userid":   madmin.Userid,
	})

	return nil
}

// RegisterHook adds a hook to the user create and destroy pipelines
func (u *userService) RegisterHook(hook Hook) {
	u.hooks = append(u.hooks, hook)
}
