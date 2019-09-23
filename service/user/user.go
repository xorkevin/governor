package user

import (
	"context"
	"fmt"
	"github.com/labstack/echo/v4"
	"strconv"
	"time"
	"xorkevin.dev/governor"
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
	moduleID        = "user"
	authRoutePrefix = "/auth"
)

type (
	// User is a user management service
	User interface {
		GetByID(userid string) (*ResUserGet, error)
		RegisterHook(hook Hook)
	}

	Service interface {
		governor.Service
		User
	}

	service struct {
		users             usermodel.Repo
		roles             rolemodel.Repo
		sessions          sessionmodel.Repo
		kvnewuser         kvstore.KVStore
		kvnewuseremail    kvstore.KVStore
		kvemailchange     kvstore.KVStore
		kvemailchangeuser kvstore.KVStore
		kvpassreset       kvstore.KVStore
		kvpassresetuser   kvstore.KVStore
		kvsessions        kvstore.KVStore
		mailer            mail.Mail
		gate              gate.Gate
		tokenizer         *token.Tokenizer
		logger            governor.Logger
		baseURL           string
		authURL           string
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
func New(users usermodel.Repo, roles rolemodel.Repo, sessions sessionmodel.Repo, kv kvstore.KVStore, mailer mail.Mail, g gate.Gate) Service {
	return &service{
		users:             users,
		roles:             roles,
		sessions:          sessions,
		kvnewuser:         kv.Subtree("newuser"),
		kvnewuseremail:    kv.Subtree("newuseremail"),
		kvemailchange:     kv.Subtree("emailchange"),
		kvemailchangeuser: kv.Subtree("emailchangeuser"),
		kvpassreset:       kv.Subtree("passreset"),
		kvpassresetuser:   kv.Subtree("passresetuser"),
		kvsessions:        kv.Subtree("sessions"),
		mailer:            mailer,
		gate:              g,
		accessTime:        time5m,
		refreshTime:       time6month,
		refreshCacheTime:  time24h,
		confirmTime:       time24h,
		passwordResetTime: time24h,
		hooks:             []Hook{},
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.baseURL = c.BaseURL
	s.authURL = c.BaseURL + r.URL() + authRoutePrefix
	if t, err := time.ParseDuration(r.GetStr("accesstime")); err != nil {
		l.Warn(fmt.Sprintf("failed to parse access time: %s", r.GetStr("accesstime")), nil)
	} else {
		s.accessTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(r.GetStr("refreshtime")); err != nil {
		l.Warn(fmt.Sprintf("failed to parse refresh time: %s", r.GetStr("refreshtime")), nil)
	} else {
		s.refreshTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(r.GetStr("refreshcache")); err != nil {
		l.Warn(fmt.Sprintf("failed to parse refresh cache: %s", r.GetStr("refreshcache")), nil)
	} else {
		s.refreshCacheTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(r.GetStr("confirmtime")); err != nil {
		l.Warn(fmt.Sprintf("failed to parse confirm time: %s", r.GetStr("confirmtime")), nil)
	} else {
		s.confirmTime = t.Nanoseconds() / b1
	}
	if t, err := time.ParseDuration(r.GetStr("passwordresettime")); err != nil {
		l.Warn(fmt.Sprintf("failed to parse password reset time: %s", r.GetStr("passwordresettime")), nil)
	} else {
		s.passwordResetTime = t.Nanoseconds() / b1
	}
	s.newLoginEmail = r.GetBool("newloginemail")
	s.passwordMinSize = r.GetInt("passwordminsize")
	if r.GetStr("secret") == "" {
		l.Warn("token secret is not set", nil)
	}
	if r.GetStr("issuer") == "" {
		l.Warn("token issuer is not set", nil)
	}
	s.tokenizer = token.New(r.GetStr("secret"), r.GetStr("issuer"))

	l.Info("loaded config", map[string]string{
		"accesstime (s)":        strconv.FormatInt(s.accessTime, 10),
		"refreshtime (s)":       strconv.FormatInt(s.refreshTime, 10),
		"refreshcache (s)":      strconv.FormatInt(s.refreshCacheTime, 10),
		"confirmtime (s)":       strconv.FormatInt(s.confirmTime, 10),
		"passwordresettime (s)": strconv.FormatInt(s.passwordResetTime, 10),
		"newloginemail":         strconv.FormatBool(s.newLoginEmail),
		"passwordminsize":       strconv.Itoa(s.passwordMinSize),
		"issuer":                r.GetStr("issuer"),
	})

	sr := s.router()
	if err := sr.mountRoute(c.IsDebug(), g.Group("/user")); err != nil {
		return err
	}
	if err := sr.mountAuth(c.IsDebug(), g.Group(authRoutePrefix)); err != nil {
		return err
	}
	l.Info("mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	madmin, err := s.users.New(req.Username, req.Password, req.Email, req.Firstname, req.Lastname, rank.Admin())
	if err != nil {
		return err
	}

	if err := s.users.Setup(); err != nil {
		return err
	}
	l.Info("created user table", nil)

	if err := s.roles.Setup(); err != nil {
		return err
	}
	l.Info("created userrole table", nil)

	if err := s.sessions.Setup(); err != nil {
		return err
	}
	l.Info("created usersession table", nil)

	if err := s.users.Insert(madmin); err != nil {
		return err
	}
	l.Info("inserted new setup admin", map[string]string{
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
