package user

import (
	"context"
	"encoding/json"
	htmlTemplate "html/template"
	"strconv"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/apikey"
	approvalmodel "xorkevin.dev/governor/service/user/approval/model"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/model"
	resetmodel "xorkevin.dev/governor/service/user/reset/model"
	"xorkevin.dev/governor/service/user/role"
	invitationmodel "xorkevin.dev/governor/service/user/role/invitation/model"
	sessionmodel "xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/hunter2"
)

const (
	authRoutePrefix = "/auth"
)

const (
	// EventStream is the backing stream for user events
	EventStream         = "DEV_XORKEVIN_GOV_USER"
	eventStreamChannels = EventStream + ".*"
	// CreateChannel is emitted when a new user is created
	CreateChannel = EventStream + ".create"
	// DeleteChannel is emitted when a user is deleted
	DeleteChannel = EventStream + ".delete"
)

const (
	time5m     int64 = int64(5 * time.Minute / time.Second)
	time24h    int64 = int64(24 * time.Hour / time.Second)
	time6month int64 = time24h * 365 / 2
)

type (
	// Users is a user management service
	Users interface {
		GetByID(userid string) (*ResUserGet, error)
		CheckUserExists(userid string) (bool, error)
		CheckUsersExist(userids []string) ([]string, error)
	}

	// Service is a Users and governor.Service
	Service interface {
		governor.Service
		Users
	}

	service struct {
		users             model.Repo
		sessions          sessionmodel.Repo
		approvals         approvalmodel.Repo
		invitations       invitationmodel.Repo
		resets            resetmodel.Repo
		roles             role.Roles
		apikeys           apikey.Apikeys
		kvusers           kvstore.KVStore
		kvsessions        kvstore.KVStore
		kvotpcodes        kvstore.KVStore
		events            events.Events
		mailer            mail.Mailer
		ratelimiter       ratelimit.Ratelimiter
		gate              gate.Gate
		tokenizer         token.Tokenizer
		otpDecrypter      *hunter2.Decrypter
		otpCipher         hunter2.Cipher
		logger            governor.Logger
		streamsize        int64
		msgsize           int32
		baseURL           string
		authURL           string
		accessTime        int64
		refreshTime       int64
		refreshCacheTime  int64
		confirmTime       int64
		passwordResetTime int64
		invitationTime    int64
		userCacheTime     int64
		newLoginEmail     bool
		passwordMinSize   int
		userApproval      bool
		rolesummary       rank.Rank
		emailurlbase      string
		otpIssuer         string
		tplemailchange    *htmlTemplate.Template
		tplforgotpass     *htmlTemplate.Template
		tplnewuser        *htmlTemplate.Template
	}

	router struct {
		s  service
		rt governor.Middleware
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

	ctxKeyUsers struct{}
)

// GetCtxUsers returns a Users service from the context
func GetCtxUsers(inj governor.Injector) Users {
	v := inj.Get(ctxKeyUsers{})
	if v == nil {
		return nil
	}
	return v.(Users)
}

// setCtxUser sets a Users service in the context
func setCtxUser(inj governor.Injector, u Users) {
	inj.Set(ctxKeyUsers{}, u)
}

// NewCtx creates a new Users service from a context
func NewCtx(inj governor.Injector) Service {
	users := model.GetCtxRepo(inj)
	sessions := sessionmodel.GetCtxRepo(inj)
	approvals := approvalmodel.GetCtxRepo(inj)
	invitations := invitationmodel.GetCtxRepo(inj)
	resets := resetmodel.GetCtxRepo(inj)
	roles := role.GetCtxRoles(inj)
	apikeys := apikey.GetCtxApikeys(inj)
	kv := kvstore.GetCtxKVStore(inj)
	ev := events.GetCtxEvents(inj)
	mailer := mail.GetCtxMailer(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	tokenizer := token.GetCtxTokenizer(inj)
	g := gate.GetCtxGate(inj)

	return New(
		users,
		sessions,
		approvals,
		invitations,
		resets,
		roles,
		apikeys,
		kv,
		ev,
		mailer,
		ratelimiter,
		tokenizer,
		g,
	)
}

// New creates a new Users service
func New(
	users model.Repo,
	sessions sessionmodel.Repo,
	approvals approvalmodel.Repo,
	invitations invitationmodel.Repo,
	resets resetmodel.Repo,
	roles role.Roles,
	apikeys apikey.Apikeys,
	kv kvstore.KVStore,
	ev events.Events,
	mailer mail.Mailer,
	ratelimiter ratelimit.Ratelimiter,
	tokenizer token.Tokenizer,
	g gate.Gate,
) Service {
	return &service{
		users:             users,
		sessions:          sessions,
		approvals:         approvals,
		invitations:       invitations,
		resets:            resets,
		roles:             roles,
		apikeys:           apikeys,
		kvusers:           kv.Subtree("users"),
		kvsessions:        kv.Subtree("sessions"),
		kvotpcodes:        kv.Subtree("otpcodes"),
		events:            ev,
		mailer:            mailer,
		ratelimiter:       ratelimiter,
		gate:              g,
		tokenizer:         tokenizer,
		accessTime:        time5m,
		refreshTime:       time6month,
		refreshCacheTime:  time24h,
		confirmTime:       time24h,
		passwordResetTime: time24h,
		invitationTime:    time24h,
		userCacheTime:     time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxUser(inj, s)

	r.SetDefault("streamsize", "200M")
	r.SetDefault("msgsize", "2K")
	r.SetDefault("accesstime", "5m")
	r.SetDefault("refreshtime", "4380h")
	r.SetDefault("refreshcache", "24h")
	r.SetDefault("confirmtime", "24h")
	r.SetDefault("passwordresettime", "24h")
	r.SetDefault("invitationtime", "24h")
	r.SetDefault("usercachetime", "24h")
	r.SetDefault("newloginemail", true)
	r.SetDefault("passwordminsize", 8)
	r.SetDefault("userapproval", false)
	r.SetDefault("otpissuer", "governor")
	r.SetDefault("rolesummary", []string{rank.TagUser, rank.TagAdmin})
	r.SetDefault("email.url.base", "http://localhost:8080")
	r.SetDefault("email.url.emailchange", "/a/confirm/email?key={{.Userid}}.{{.Key}}")
	r.SetDefault("email.url.forgotpass", "/x/resetpass?key={{.Userid}}.{{.Key}}")
	r.SetDefault("email.url.newuser", "/x/confirm?userid={{.Userid}}&key={{.Key}}")
}

func (s *service) router() *router {
	return &router{
		s: *s,
		rt: ratelimit.Compose(
			s.ratelimiter,
			ratelimit.IPAddress("ip", 60, 15, 240),
			ratelimit.Userid("id", 60, 15, 240),
			ratelimit.UseridIPAddress("id_ip", 60, 15, 120),
		),
	}
}

type (
	secretOTP struct {
		Keys []string `mapstructure:"secrets"`
	}
)

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid stream size")
	}
	msgsize, err := bytefmt.ToBytes(r.GetStr("msgsize"))
	if err != nil {
		return governor.ErrWithMsg(err, "Invalid msg size")
	}
	s.msgsize = int32(msgsize)
	s.baseURL = c.BaseURL
	s.authURL = c.BaseURL + r.URL() + authRoutePrefix
	if t, err := time.ParseDuration(r.GetStr("accesstime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse access time")
	} else {
		s.accessTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("refreshtime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse refresh time")
	} else {
		s.refreshTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("refreshcache")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse refresh cache")
	} else {
		s.refreshCacheTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("confirmtime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse confirm time")
	} else {
		s.confirmTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("passwordresettime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse password reset time")
	} else {
		s.passwordResetTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("invitationtime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse role invitation time")
	} else {
		s.invitationTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("usercachetime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse user cache time")
	} else {
		s.userCacheTime = int64(t / time.Second)
	}
	s.newLoginEmail = r.GetBool("newloginemail")
	s.passwordMinSize = r.GetInt("passwordminsize")
	s.userApproval = r.GetBool("userapproval")
	s.otpIssuer = r.GetStr("otpissuer")
	s.rolesummary = rank.FromSlice(r.GetStrSlice("rolesummary"))

	s.emailurlbase = r.GetStr("email.url.base")
	if t, err := htmlTemplate.New("email.url.emailchange").Parse(r.GetStr("email.url.emailchange")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse email change url template")
	} else {
		s.tplemailchange = t
	}
	if t, err := htmlTemplate.New("email.url.forgotpass").Parse(r.GetStr("email.url.forgotpass")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse forgot pass url template")
	} else {
		s.tplforgotpass = t
	}
	if t, err := htmlTemplate.New("email.url.newuser").Parse(r.GetStr("email.url.newuser")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse new user url template")
	} else {
		s.tplnewuser = t
	}

	otpsecrets := secretOTP{}
	if err := r.GetSecret("otpkey", 0, &otpsecrets); err != nil {
		return governor.ErrWithMsg(err, "Invalid otpkey secrets")
	}
	if len(otpsecrets.Keys) == 0 {
		return governor.ErrWithKind(nil, governor.ErrInvalidConfig{}, "No otpkey present")
	}
	s.otpDecrypter = hunter2.NewDecrypter()
	for n, i := range otpsecrets.Keys {
		cipher, err := hunter2.CipherFromParams(i, hunter2.DefaultCipherAlgs)
		if err != nil {
			return governor.ErrWithKind(err, governor.ErrInvalidConfig{}, "Invalid cipher param")
		}
		if n == 0 {
			s.otpCipher = cipher
		}
		s.otpDecrypter.RegisterCipher(cipher)
	}

	l.Info("loaded config", map[string]string{
		"stream size (bytes)":   r.GetStr("streamsize"),
		"msg size (bytes)":      r.GetStr("msgsize"),
		"accesstime (s)":        strconv.FormatInt(s.accessTime, 10),
		"refreshtime (s)":       strconv.FormatInt(s.refreshTime, 10),
		"refreshcache (s)":      strconv.FormatInt(s.refreshCacheTime, 10),
		"confirmtime (s)":       strconv.FormatInt(s.confirmTime, 10),
		"passwordresettime (s)": strconv.FormatInt(s.passwordResetTime, 10),
		"invitationtime (s)":    strconv.FormatInt(s.invitationTime, 10),
		"usercachetime (s)":     strconv.FormatInt(s.userCacheTime, 10),
		"newloginemail":         strconv.FormatBool(s.newLoginEmail),
		"passwordminsize":       strconv.Itoa(s.passwordMinSize),
		"userapproval":          strconv.FormatBool(s.userApproval),
		"otpissuer":             s.otpIssuer,
		"numotpkeys":            strconv.Itoa(len(otpsecrets.Keys)),
		"rolesummary":           s.rolesummary.String(),
		"tplemailchange":        r.GetStr("email.url.emailchange"),
		"tplforgotpass":         r.GetStr("email.url.forgotpass"),
		"tplnewuser":            r.GetStr("email.url.newuser"),
	})

	sr := s.router()
	sr.mountRoute(m.Group("/user"))
	sr.mountAuth(m.Group(authRoutePrefix))
	sr.mountApikey(m.Group("/apikey"))
	l.Info("mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.events.InitStream(EventStream, []string{eventStreamChannels}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.msgsize,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to init user stream")
	}
	l.Info("Created user stream", nil)

	if err := s.users.Setup(); err != nil {
		return err
	}
	l.Info("created user table", nil)

	if err := s.sessions.Setup(); err != nil {
		return err
	}
	l.Info("created usersessions table", nil)

	if err := s.approvals.Setup(); err != nil {
		return err
	}
	l.Info("created userapprovals table", nil)

	if err := s.invitations.Setup(); err != nil {
		return err
	}
	l.Info("created userroleinvitations table", nil)

	if err := s.resets.Setup(); err != nil {
		return err
	}
	l.Info("created userresets table", nil)

	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "postsetup",
	})

	if a := req.Admin; req.First && a != nil {
		madmin, err := s.users.New(a.Username, a.Password, a.Email, a.Firstname, a.Lastname)
		if err != nil {
			return err
		}

		b, err := json.Marshal(NewUserProps{
			Userid:       madmin.Userid,
			Username:     madmin.Username,
			Email:        madmin.Email,
			FirstName:    madmin.FirstName,
			LastName:     madmin.LastName,
			CreationTime: madmin.CreationTime,
		})
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to encode admin user props to json")
		}

		if err := s.users.Insert(madmin); err != nil {
			return err
		}
		if err := s.roles.InsertRoles(madmin.Userid, rank.Admin()); err != nil {
			return err
		}

		if err := s.events.StreamPublish(CreateChannel, b); err != nil {
			s.logger.Error("Failed to publish new user", map[string]string{
				"error":      err.Error(),
				"actiontype": "publishadminuser",
			})
		}

		l.Info("inserted new setup admin", map[string]string{
			"username": madmin.Username,
			"userid":   madmin.Userid,
		})
	}

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
