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
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/service/user/approval/model"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/hunter2"
)

const (
	authRoutePrefix = "/auth"
)

type (
	// User is a user management service
	User interface {
		GetByID(userid string) (*ResUserGet, error)
	}

	Service interface {
		governor.Service
		User
	}

	service struct {
		users             usermodel.Repo
		sessions          sessionmodel.Repo
		approvals         approvalmodel.Repo
		apikeys           apikeymodel.Repo
		roles             role.Role
		kvnewuser         kvstore.KVStore
		kvemailchange     kvstore.KVStore
		kvpassreset       kvstore.KVStore
		kvsessions        kvstore.KVStore
		queue             msgqueue.Msgqueue
		mailer            mail.Mail
		gate              gate.Gate
		hasher            *hunter2.Blake2bHasher
		verifier          *hunter2.Verifier
		tokenizer         token.Tokenizer
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
		userApproval      bool
	}

	router struct {
		s service
	}

	// NewUserProps are properties of a newly created user
	NewUserProps struct {
		Userid       string `json:"userid"`
		Username     string `json:"username"`
		Email        string `json:"email"`
		FirstName    string `json:"first_name"`
		LastName     string `json:"last_name"`
		CreationTime int64  `json:"creation_time"`
	}

	// DeleteUserProps are properties of a deleted user
	DeleteUserProps struct {
		Userid string `json:"userid"`
	}
)

const (
	NewUserQueueID    = "gov.user.new"
	DeleteUserQueueID = "gov.user.delete"
)

const (
	time5m     int64 = 300
	time24h    int64 = 86400
	time6month int64 = time24h * 365 / 2
	b1               = 1_000_000_000
)

// New creates a new User
func New(users usermodel.Repo, sessions sessionmodel.Repo, approvals approvalmodel.Repo, apikeys apikeymodel.Repo, roles role.Role, kv kvstore.KVStore, queue msgqueue.Msgqueue, mailer mail.Mail, tokenizer token.Tokenizer, g gate.Gate) Service {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &service{
		users:             users,
		sessions:          sessions,
		approvals:         approvals,
		apikeys:           apikeys,
		roles:             roles,
		kvnewuser:         kv.Subtree("newuser"),
		kvemailchange:     kv.Subtree("emailchange"),
		kvpassreset:       kv.Subtree("passreset"),
		kvsessions:        kv.Subtree("sessions"),
		queue:             queue,
		mailer:            mailer,
		gate:              g,
		hasher:            hasher,
		verifier:          verifier,
		tokenizer:         tokenizer,
		accessTime:        time5m,
		refreshTime:       time6month,
		refreshCacheTime:  time24h,
		confirmTime:       time24h,
		passwordResetTime: time24h,
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
	r.SetDefault("userapproval", false)
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
	s.userApproval = r.GetBool("userapproval")

	l.Info("loaded config", map[string]string{
		"accesstime (s)":        strconv.FormatInt(s.accessTime, 10),
		"refreshtime (s)":       strconv.FormatInt(s.refreshTime, 10),
		"refreshcache (s)":      strconv.FormatInt(s.refreshCacheTime, 10),
		"confirmtime (s)":       strconv.FormatInt(s.confirmTime, 10),
		"passwordresettime (s)": strconv.FormatInt(s.passwordResetTime, 10),
		"newloginemail":         strconv.FormatBool(s.newLoginEmail),
		"passwordminsize":       strconv.Itoa(s.passwordMinSize),
		"issuer":                r.GetStr("issuer"),
		"userapproval":          strconv.FormatBool(s.userApproval),
	})

	sr := s.router()
	sr.mountRoute(g.Group("/user"))
	sr.mountAuth(g.Group(authRoutePrefix))
	sr.mountApikey(g.Group("/apikey"))
	l.Info("mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	madmin, err := s.users.New(req.Username, req.Password, req.Email, req.Firstname, req.Lastname)
	if err != nil {
		return err
	}

	if err := s.users.Setup(); err != nil {
		return err
	}
	l.Info("created user table", nil)

	if err := s.sessions.Setup(); err != nil {
		return err
	}
	l.Info("created usersession table", nil)

	if err := s.approvals.Setup(); err != nil {
		return err
	}
	l.Info("created userapprovals table", nil)

	if err := s.apikeys.Setup(); err != nil {
		return err
	}
	l.Info("created userapikeys table", nil)

	if err := s.users.Insert(madmin); err != nil {
		return err
	}
	if err := s.roles.InsertRoles(madmin.Userid, rank.Admin()); err != nil {
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
