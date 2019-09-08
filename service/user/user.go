package user

import (
	"context"
	"fmt"
	"github.com/labstack/echo"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/kvstore"
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
		RegisterHook(hook Hook)
	}

	// Service is the public interface for the user service server
	Service interface {
		governor.Service
		User
	}

	service struct {
		users             usermodel.Repo
		roles             rolemodel.Repo
		sessions          sessionmodel.Repo
		kv                kvstore.KVStore
		mailer            mail.Mail
		gate              gate.Gate
		cc                cachecontrol.CacheControl
		tokenizer         *token.Tokenizer
		logger            governor.Logger
		accessTime        int64
		refreshTime       int64
		refreshCacheTime  int64
		confirmTime       int64
		passwordResetTime int64
		newLoginEmail     bool
		passwordMinSize   int
		hooks             []Hook
	}

	router struct {
		s service
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
	time5m     int64 = 300
	time24h    int64 = 86400
	time6month int64 = time24h * 365 / 2
	b1               = 1_000_000_000
)

// New creates a new User
func New(users usermodel.Repo, roles rolemodel.Repo, sessions sessionmodel.Repo, kv kvstore.KVStore, mailer mail.Mail, g gate.Gate, cc cachecontrol.CacheControl) Service {
	return &service{
		users:             users,
		roles:             roles,
		sessions:          sessions,
		kv:                kv,
		mailer:            mailer,
		gate:              g,
		cc:                cc,
		accessTime:        time5m,
		refreshTime:       time6month,
		refreshCacheTime:  time24h,
		confirmTime:       time24h,
		passwordResetTime: time24h,
		hooks:             []Hook{},
	}
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("accesstime", "5m")
	r.SetDefault("refreshtime", "4380h")
	r.SetDefault("refreshcache", "24h")
	r.SetDefault("confirmtime", "24h")
	r.SetDefault("passwordresettime", "24h")
	r.SetDefault("newloginemail", true)
	r.SetDefault("passwordminsize", 8)
	r.SetDefault("secret", "")
	r.SetDefault("issuer", "governor")
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	conf := r.GetStrMap("")

	if t, err := time.ParseDuration(conf["accesstime"]); err != nil {
		l.Warn(fmt.Sprintf("user: fail to parse access time: %s", conf["accesstime"]), nil)
	} else {
		s.accessTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(conf["refreshtime"]); err != nil {
		l.Warn(fmt.Sprintf("user: fail to parse refresh time: %s", conf["refreshtime"]), nil)
	} else {
		s.refreshTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(conf["refreshcache"]); err != nil {
		l.Warn(fmt.Sprintf("user: fail to parse refresh cache: %s", conf["refreshcache"]), nil)
	} else {
		s.refreshCacheTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(conf["confirmtime"]); err != nil {
		l.Warn(fmt.Sprintf("user: fail to parse confirm time: %s", conf["confirmtime"]), nil)
	} else {
		s.confirmTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(conf["passwordresettime"]); err != nil {
		l.Warn(fmt.Sprintf("user: fail to parse password reset time: %s", conf["passwordresettime"]), nil)
	} else {
		s.passwordResetTime = t.Nanoseconds() / b1
	}
	s.newLoginEmail = r.GetBool("newloginemail")
	s.passwordMinSize = r.GetInt("passwordminsize")
	if conf["secret"] == "" {
		s.logger.Warn("gate: token secret is not set", nil)
	}
	if conf["issuer"] == "" {
		s.logger.Warn("gate: token issuer is not set", nil)
	}
	s.tokenizer = token.New(conf["secret"], conf["issuer"])

	l.Info("init user service", map[string]string{
		"user: accesstime (s)":        strconv.FormatInt(s.accessTime, 10),
		"user: refreshtime (s)":       strconv.FormatInt(s.refreshTime, 10),
		"user: refreshcache (s)":      strconv.FormatInt(s.refreshCacheTime, 10),
		"user: confirmtime (s)":       strconv.FormatInt(s.confirmTime, 10),
		"user: passwordresettime (s)": strconv.FormatInt(s.passwordResetTime, 10),
		"user: newloginemail":         strconv.FormatBool(s.newLoginEmail),
		"user: passwordminsize":       strconv.Itoa(s.passwordMinSize),
		"user: issuer":                conf["issuer"],
	})

	router := s.router()
	if err := router.mountRoute(c, g.Group("/user")); err != nil {
		return err
	}
	if err := router.mountAuth(conf, g.Group("/auth")); err != nil {
		return err
	}
	l.Info("user: mount http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	madmin, err := s.users.New(req.Username, req.Password, req.Email, req.Firstname, req.Lastname, rank.Admin())
	if err != nil {
		return err
	}

	if err := s.users.Setup(); err != nil {
		return err
	}
	s.logger.Info("create user table", nil)

	if err := s.roles.Setup(); err != nil {
		return err
	}
	s.logger.Info("create userrole table", nil)

	if err := s.sessions.Setup(); err != nil {
		return err
	}
	s.logger.Info("create usersession table", nil)

	if err := s.users.Insert(madmin); err != nil {
		return err
	}
	s.logger.Info("insert new setup admin", map[string]string{
		"username": madmin.Username,
		"userid":   madmin.Userid,
	})
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// RegisterHook adds a hook to the user create and destroy pipelines
func (s *service) RegisterHook(hook Hook) {
	s.hooks = append(s.hooks, hook)
}
