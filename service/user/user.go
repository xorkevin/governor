package user

import (
	"context"
	htmlTemplate "html/template"
	"net/http"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/user/apikey"
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
		roles             role.Role
		apikeys           apikey.Apikey
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
		emailurlbase      string
		tplemailchange    *htmlTemplate.Template
		tplforgotpass     *htmlTemplate.Template
		tplnewuser        *htmlTemplate.Template
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
	time5m     int64 = int64(5 * time.Minute / time.Second)
	time24h    int64 = int64(24 * time.Hour / time.Second)
	time6month int64 = time24h * 365 / 2
)

// New creates a new User
func New(users usermodel.Repo, sessions sessionmodel.Repo, approvals approvalmodel.Repo, roles role.Role, apikeys apikey.Apikey, kv kvstore.KVStore, queue msgqueue.Msgqueue, mailer mail.Mail, tokenizer token.Tokenizer, g gate.Gate) Service {
	hasher := hunter2.NewBlake2bHasher()
	verifier := hunter2.NewVerifier()
	verifier.RegisterHash(hasher)

	return &service{
		users:             users,
		sessions:          sessions,
		approvals:         approvals,
		roles:             roles,
		apikeys:           apikeys,
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
	r.SetDefault("email.url.base", "http://localhost:8080")
	r.SetDefault("email.url.emailchange", "/a/account/email/confirm?key={{.Userid}}.{{.Key}}")
	r.SetDefault("email.url.forgotpass", "/x/resetpass?key={{ .Userid }}.{{ .Key }}")
	r.SetDefault("email.url.newuser", "/x/confirm?email={{ .Email }}&key={{ .Key }}")
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	s.baseURL = c.BaseURL
	s.authURL = c.BaseURL + r.URL() + authRoutePrefix
	if t, err := time.ParseDuration(r.GetStr("accesstime")); err != nil {
		return governor.NewError("Failed to parse access time", http.StatusBadRequest, err)
	} else {
		s.accessTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("refreshtime")); err != nil {
		return governor.NewError("Failed to parse refresh time", http.StatusBadRequest, err)
	} else {
		s.refreshTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("refreshcache")); err != nil {
		return governor.NewError("Failed to parse refresh cache", http.StatusBadRequest, err)
	} else {
		s.refreshCacheTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("confirmtime")); err != nil {
		return governor.NewError("Failed to parse confirm time", http.StatusBadRequest, err)
	} else {
		s.confirmTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("passwordresettime")); err != nil {
		return governor.NewError("Failed to parse password reset time", http.StatusBadRequest, err)
	} else {
		s.passwordResetTime = int64(t / time.Second)
	}
	s.newLoginEmail = r.GetBool("newloginemail")
	s.passwordMinSize = r.GetInt("passwordminsize")
	s.userApproval = r.GetBool("userapproval")

	s.emailurlbase = r.GetStr("email.url.base")
	if t, err := htmlTemplate.New("email.url.emailchange").Parse(r.GetStr("email.url.emailchange")); err != nil {
		return governor.NewError("Failed to parse email change url template", http.StatusBadRequest, err)
	} else {
		s.tplemailchange = t
	}
	if t, err := htmlTemplate.New("email.url.forgotpass").Parse(r.GetStr("email.url.forgotpass")); err != nil {
		return governor.NewError("Failed to parse forgot pass url template", http.StatusBadRequest, err)
	} else {
		s.tplforgotpass = t
	}
	if t, err := htmlTemplate.New("email.url.newuser").Parse(r.GetStr("email.url.newuser")); err != nil {
		return governor.NewError("Failed to parse new user url template", http.StatusBadRequest, err)
	} else {
		s.tplnewuser = t
	}

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
		"tplemailchange":        r.GetStr("email.url.emailchange"),
		"tplforgotpass":         r.GetStr("email.url.forgotpass"),
		"tplnewuser":            r.GetStr("email.url.newuser"),
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
