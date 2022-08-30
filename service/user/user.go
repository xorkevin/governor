package user

import (
	"context"
	"encoding/json"
	htmlTemplate "html/template"
	"strconv"
	"strings"
	"sync/atomic"
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
	"xorkevin.dev/kerrors"
)

const (
	authRoutePrefix = "/auth"
)

const (
	time5m     int64 = int64(5 * time.Minute / time.Second)
	time24h    int64 = int64(24 * time.Hour / time.Second)
	time72h    int64 = time24h * 3
	time6month int64 = time24h * 365 / 2
)

type (
	// Users is a user management service
	Users interface {
		GetByID(ctx context.Context, userid string) (*ResUserGet, error)
		GetByUsername(ctx context.Context, username string) (*ResUserGet, error)
		GetByEmail(ctx context.Context, email string) (*ResUserGet, error)
		GetInfoBulk(ctx context.Context, userids []string) (*ResUserInfoList, error)
		CheckUserExists(ctx context.Context, userid string) (bool, error)
		CheckUsersExist(ctx context.Context, userids []string) ([]string, error)
		DeleteRoleInvitations(ctx context.Context, role string) error
	}

	// Service is a Users and governor.Service
	Service interface {
		governor.Service
		Users
	}

	otpCipher struct {
		cipher    hunter2.Cipher
		decrypter *hunter2.Decrypter
	}

	tplName struct {
		emailchange       string
		emailchangenotify string
		passchange        string
		forgotpass        string
		passreset         string
		loginratelimit    string
		otpbackupused     string
		newuser           string
	}

	emailURLTpl struct {
		base        string
		emailchange *htmlTemplate.Template
		forgotpass  *htmlTemplate.Template
		newuser     *htmlTemplate.Template
	}

	getCipherRes struct {
		cipher *otpCipher
		err    error
	}

	getOp struct {
		ctx context.Context
		res chan<- getCipherRes
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
		otpCipher         *otpCipher
		aotpCipher        *atomic.Pointer[otpCipher]
		config            governor.SecretReader
		logger            governor.Logger
		rolens            string
		scopens           string
		streamns          string
		opts              Opts
		streamsize        int64
		eventsize         int32
		baseURL           string
		authURL           string
		accessTime        int64
		refreshTime       int64
		refreshCacheTime  int64
		confirmTime       int64
		passwordReset     bool
		passwordResetTime int64
		passResetDelay    int64
		invitationTime    int64
		userCacheTime     int64
		newLoginEmail     bool
		passwordMinSize   int
		userApproval      bool
		otpIssuer         string
		rolesummary       rank.Rank
		tplname           tplName
		emailurl          emailURLTpl
		ops               chan getOp
		ready             *atomic.Bool
		hbfailed          int
		hbinterval        int
		hbmaxfail         int
		done              <-chan struct{}
		otprefresh        int
		syschannels       governor.SysChannels
	}

	router struct {
		s  *service
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

	// UpdateUserProps are properties of a user update
	UpdateUserProps struct {
		Userid   string `json:"userid"`
		Username string `json:"username"`
	}

	ctxKeyUsers struct{}

	Opts struct {
		StreamName    string
		CreateChannel string
		DeleteChannel string
		UpdateChannel string
	}

	ctxKeyOpts struct{}
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

// GetCtxOpts returns user Opts from the context
func GetCtxOpts(inj governor.Injector) Opts {
	v := inj.Get(ctxKeyOpts{})
	if v == nil {
		return Opts{}
	}
	return v.(Opts)
}

// SetCtxOpts sets user Opts in the context
func SetCtxOpts(inj governor.Injector, o Opts) {
	inj.Set(ctxKeyOpts{}, o)
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
		aotpCipher:        &atomic.Pointer[otpCipher]{},
		accessTime:        time5m,
		refreshTime:       time6month,
		refreshCacheTime:  time24h,
		confirmTime:       time24h,
		passwordReset:     true,
		passwordResetTime: time24h,
		passResetDelay:    0,
		invitationTime:    time24h,
		userCacheTime:     time24h,
		ops:               make(chan getOp),
		ready:             &atomic.Bool{},
		hbfailed:          0,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxUser(inj, s)
	s.rolens = "gov." + name
	s.scopens = "gov." + name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = Opts{
		StreamName:    streamname,
		CreateChannel: streamname + ".create",
		DeleteChannel: streamname + ".delete",
		UpdateChannel: streamname + ".update",
	}
	SetCtxOpts(inj, s.opts)

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("accesstime", "5m")
	r.SetDefault("refreshtime", "4380h")
	r.SetDefault("refreshcache", "24h")
	r.SetDefault("confirmtime", "24h")
	r.SetDefault("passwordreset", true)
	r.SetDefault("passwordresettime", "24h")
	r.SetDefault("passresetdelay", 0)
	r.SetDefault("invitationtime", "24h")
	r.SetDefault("usercachetime", "24h")
	r.SetDefault("newloginemail", true)
	r.SetDefault("passwordminsize", 8)
	r.SetDefault("userapproval", false)
	r.SetDefault("otpissuer", "governor")
	r.SetDefault("rolesummary", []string{rank.TagUser, rank.TagAdmin})
	r.SetDefault("email.tpl.emailchange", "emailchange")
	r.SetDefault("email.tpl.emailchangenotify", "emailchangenotify")
	r.SetDefault("email.tpl.passchange", "passchange")
	r.SetDefault("email.tpl.forgotpass", "forgotpass")
	r.SetDefault("email.tpl.passreset", "passreset")
	r.SetDefault("email.tpl.loginratelimit", "loginratelimit")
	r.SetDefault("email.tpl.otpbackupused", "otpbackupused")
	r.SetDefault("email.tpl.newuser", "newuser")
	r.SetDefault("email.url.base", "http://localhost:8080")
	r.SetDefault("email.url.emailchange", "/a/confirm/email?key={{.Userid}}.{{.Key}}")
	r.SetDefault("email.url.forgotpass", "/x/resetpass?key={{.Userid}}.{{.Key}}")
	r.SetDefault("email.url.newuser", "/x/confirm?userid={{.Userid}}&key={{.Key}}")
	r.SetDefault("hbinterval", 5)
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("otprefresh", 60)
}

func (s *service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.Base(),
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

	s.config = r

	var err error
	s.streamsize, err = bytefmt.ToBytes(r.GetStr("streamsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid stream size")
	}
	eventsize, err := bytefmt.ToBytes(r.GetStr("eventsize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid msg size")
	}
	s.eventsize = int32(eventsize)
	s.baseURL = c.BaseURL
	s.authURL = c.BaseURL + r.URL() + authRoutePrefix
	if t, err := time.ParseDuration(r.GetStr("accesstime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse access time")
	} else {
		s.accessTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("refreshtime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse refresh time")
	} else {
		s.refreshTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("refreshcache")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse refresh cache")
	} else {
		s.refreshCacheTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("confirmtime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse confirm time")
	} else {
		s.confirmTime = int64(t / time.Second)
	}
	s.passwordReset = r.GetBool("passwordreset")
	if t, err := time.ParseDuration(r.GetStr("passwordresettime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse password reset time")
	} else {
		s.passwordResetTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("passresetdelay")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse password reset delay")
	} else {
		s.passResetDelay = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("invitationtime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse role invitation time")
	} else {
		s.invitationTime = int64(t / time.Second)
	}
	if t, err := time.ParseDuration(r.GetStr("usercachetime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse user cache time")
	} else {
		s.userCacheTime = int64(t / time.Second)
	}
	s.newLoginEmail = r.GetBool("newloginemail")
	s.passwordMinSize = r.GetInt("passwordminsize")
	s.userApproval = r.GetBool("userapproval")
	s.otpIssuer = r.GetStr("otpissuer")
	s.rolesummary = rank.FromSlice(r.GetStrSlice("rolesummary"))

	s.tplname = tplName{
		emailchange:       r.GetStr("email.tpl.emailchange"),
		emailchangenotify: r.GetStr("email.tpl.emailchangenotify"),
		passchange:        r.GetStr("email.tpl.passchange"),
		forgotpass:        r.GetStr("email.tpl.forgotpass"),
		passreset:         r.GetStr("email.tpl.passreset"),
		loginratelimit:    r.GetStr("email.tpl.loginratelimit"),
		otpbackupused:     r.GetStr("email.tpl.otpbackupused"),
		newuser:           r.GetStr("email.tpl.newuser"),
	}

	s.emailurl.base = r.GetStr("email.url.base")
	if t, err := htmlTemplate.New("email.url.emailchange").Parse(r.GetStr("email.url.emailchange")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse email change url template")
	} else {
		s.emailurl.emailchange = t
	}
	if t, err := htmlTemplate.New("email.url.forgotpass").Parse(r.GetStr("email.url.forgotpass")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse forgot pass url template")
	} else {
		s.emailurl.forgotpass = t
	}
	if t, err := htmlTemplate.New("email.url.newuser").Parse(r.GetStr("email.url.newuser")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse new user url template")
	} else {
		s.emailurl.newuser = t
	}

	s.hbinterval = r.GetInt("hbinterval")
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.otprefresh = r.GetInt("otprefresh")

	s.syschannels = c.SysChannels

	l.Info("Loaded config", map[string]string{
		"stream size (bytes)":      r.GetStr("streamsize"),
		"event size (bytes)":       r.GetStr("eventsize"),
		"accesstime (s)":           strconv.FormatInt(s.accessTime, 10),
		"refreshtime (s)":          strconv.FormatInt(s.refreshTime, 10),
		"refreshcache (s)":         strconv.FormatInt(s.refreshCacheTime, 10),
		"confirmtime (s)":          strconv.FormatInt(s.confirmTime, 10),
		"passwordreset":            strconv.FormatBool(s.passwordReset),
		"passwordresettime (s)":    strconv.FormatInt(s.passwordResetTime, 10),
		"passresetdelay (s)":       strconv.FormatInt(s.passResetDelay, 10),
		"invitationtime (s)":       strconv.FormatInt(s.invitationTime, 10),
		"usercachetime (s)":        strconv.FormatInt(s.userCacheTime, 10),
		"newloginemail":            strconv.FormatBool(s.newLoginEmail),
		"passwordminsize":          strconv.Itoa(s.passwordMinSize),
		"userapproval":             strconv.FormatBool(s.userApproval),
		"otpissuer":                s.otpIssuer,
		"rolesummary":              s.rolesummary.String(),
		"tplnameemailchange":       s.tplname.emailchange,
		"tplnameemailchangenotify": s.tplname.emailchangenotify,
		"tplnamepasschange":        s.tplname.passchange,
		"tplnameforgotpass":        s.tplname.forgotpass,
		"tplnamepassreset":         s.tplname.passreset,
		"tplnameloginratelimit":    s.tplname.loginratelimit,
		"tplnameotpbackupused":     s.tplname.otpbackupused,
		"emailurlbase":             s.emailurl.base,
		"tplemailchange":           r.GetStr("email.url.emailchange"),
		"tplforgotpass":            r.GetStr("email.url.forgotpass"),
		"tplnewuser":               r.GetStr("email.url.newuser"),
		"hbinterval":               strconv.Itoa(s.hbinterval),
		"hbmaxfail":                strconv.Itoa(s.hbmaxfail),
		"otprefresh":               strconv.Itoa(s.otprefresh),
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	sr := s.router()
	sr.mountRoute(m.Group("/user"))
	sr.mountAuth(m.Group(authRoutePrefix))
	sr.mountApikey(m.Group("/apikey"))
	l.Info("Mounted http routes", nil)
	return nil
}

func (s *service) execute(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	ticker := time.NewTicker(time.Duration(s.hbinterval) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.handlePing(ctx)
		case op := <-s.ops:
			cipher, err := s.handleGetCipher(ctx)
			select {
			case <-op.ctx.Done():
			case op.res <- getCipherRes{
				cipher: cipher,
				err:    err,
			}:
				close(op.res)
			}
		}
	}
}

func (s *service) handlePing(ctx context.Context) {
	err := s.refreshSecrets(ctx)
	if err == nil {
		s.ready.Store(true)
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.logger.Warn("Failed to refresh otp secrets", map[string]string{
			"error":      err.Error(),
			"actiontype": "user_refresh_otp_secrets",
		})
		return
	}
	s.logger.Error("Failed max refresh attempts", map[string]string{
		"error":      err.Error(),
		"actiontype": "user_refresh_otp_secrets",
	})
	s.ready.Store(false)
	s.hbfailed = 0
}

func (s *service) refreshSecrets(ctx context.Context) error {
	var otpsecrets secretOTP
	if err := s.config.GetSecret(ctx, "otpkey", int64(s.otprefresh), &otpsecrets); err != nil {
		return kerrors.WithMsg(err, "Invalid otpkey secrets")
	}
	if len(otpsecrets.Keys) == 0 {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "No otpkey present")
	}
	decrypter := hunter2.NewDecrypter()
	var cipher hunter2.Cipher
	for n, i := range otpsecrets.Keys {
		c, err := hunter2.CipherFromParams(i, hunter2.DefaultCipherAlgs)
		if err != nil {
			return kerrors.WithKind(err, governor.ErrInvalidConfig{}, "Invalid cipher param")
		}
		if n == 0 {
			if s.otpCipher != nil && s.otpCipher.cipher.ID() == c.ID() {
				// first, newest cipher matches current cipher, therefore no change in ciphers
				return nil
			}
			cipher = c
		}
		decrypter.RegisterCipher(c)
	}
	s.otpCipher = &otpCipher{
		cipher:    cipher,
		decrypter: decrypter,
	}
	s.aotpCipher.Store(s.otpCipher)
	s.logger.Info("Refreshed otp secrets with new secrets", map[string]string{
		"actiontype": "user_refresh_otp_secrets",
		"kid":        s.otpCipher.cipher.ID(),
		"numotpkeys": strconv.Itoa(len(otpsecrets.Keys)),
	})
	return nil
}

func (s *service) handleGetCipher(ctx context.Context) (*otpCipher, error) {
	if s.otpCipher == nil {
		if err := s.refreshSecrets(ctx); err != nil {
			return nil, err
		}
		s.ready.Store(true)
	}
	return s.otpCipher, nil
}

func (s *service) getCipher(ctx context.Context) (*otpCipher, error) {
	if cipher := s.aotpCipher.Load(); cipher != nil {
		return cipher, nil
	}

	res := make(chan getCipherRes)
	op := getOp{
		ctx: ctx,
		res: res,
	}
	select {
	case <-s.done:
		return nil, kerrors.WithMsg(nil, "User service shutdown")
	case <-ctx.Done():
		return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
	case s.ops <- op:
		select {
		case <-ctx.Done():
			return nil, kerrors.WithMsg(ctx.Err(), "Context cancelled")
		case v := <-res:
			return v.cipher, v.err
		}
	}
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.events.InitStream(context.Background(), s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init user stream")
	}
	l.Info("Created user stream", nil)

	if err := s.users.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created user table", nil)

	if err := s.sessions.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created usersessions table", nil)

	if err := s.approvals.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created userapprovals table", nil)

	if err := s.invitations.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created userroleinvitations table", nil)

	if err := s.resets.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created userresets table", nil)

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
			return kerrors.WithMsg(err, "Failed to encode admin user props to json")
		}

		if err := s.users.Insert(context.Background(), madmin); err != nil {
			return err
		}
		if err := s.roles.InsertRoles(context.Background(), madmin.Userid, rank.Admin()); err != nil {
			return err
		}

		if err := s.events.StreamPublish(context.Background(), s.opts.CreateChannel, b); err != nil {
			s.logger.Error("Failed to publish new user", map[string]string{
				"error":      err.Error(),
				"actiontype": "user_publish_create_admin",
			})
		}

		l.Info("Created admin from setup", map[string]string{
			"actiontype": "user_create_admin",
			"username":   madmin.Username,
			"userid":     madmin.Userid,
		})
	}

	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DeleteChannel, s.streamns+"_WORKER_ROLE_DELETE", s.UserRoleDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)

	if _, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DeleteChannel, s.streamns+"_WORKER_APIKEY_DELETE", s.UserApikeyDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)

	if _, err := s.events.Subscribe(s.syschannels.GC, s.streamns+"_WORKER_APPROVAL_GC", s.UserApprovalGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	l.Info("Subscribed to gov sys gc channel", nil)

	if _, err := s.events.Subscribe(s.syschannels.GC, s.streamns+"_WORKER_RESET_GC", s.UserResetGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	l.Info("Subscribed to gov sys gc channel", nil)

	if _, err := s.events.Subscribe(s.syschannels.GC, s.streamns+"_WORKER_INVITATION_GC", s.UserInvitationGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	l.Info("Subscribed to gov sys gc channel", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		l.Warn("Failed to stop", map[string]string{
			"error":      ctx.Err().Error(),
			"actiontype": "user_stop",
		})
	}
}

func (s *service) Health() error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig{}, "User service not ready")
	}
	return nil
}

const (
	roleDeleteBatchSize = 256
)

// UserRoleDeleteHook deletes the roles of a deleted user
func (s *service) UserRoleDeleteHook(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		r, err := s.roles.GetRoles(ctx, props.Userid, "", roleDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user roles")
		}
		if len(r) == 0 {
			break
		}
		if err := s.roles.DeleteRoles(ctx, props.Userid, r); err != nil {
			return kerrors.WithMsg(err, "Failed to delete user roles")
		}
		if len(r) < roleDeleteBatchSize {
			break
		}
	}
	return nil
}

const (
	apikeyDeleteBatchSize = 256
)

// UserApikeyDeleteHook deletes the apikeys of a deleted user
func (s *service) UserApikeyDeleteHook(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		keys, err := s.apikeys.GetUserKeys(ctx, props.Userid, apikeyDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user apikeys")
		}
		if len(keys) == 0 {
			break
		}
		keyids := make([]string, 0, len(keys))
		for _, i := range keys {
			keyids = append(keyids, i.Keyid)
		}
		if err := s.apikeys.DeleteKeys(ctx, keyids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete user apikeys")
		}
		if len(keys) < apikeyDeleteBatchSize {
			break
		}
	}
	return nil
}

func (s *service) UserApprovalGCHook(ctx context.Context, topic string, msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": s.syschannels.GC,
		"group":   s.streamns + "_WORKER_APPROVAL_GC",
	})
	props, err := governor.DecodeSysEventTimestampProps(msgdata)
	if err != nil {
		l.Error(err.Error(), nil)
		return
	}
	if err := s.approvals.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		l.Error(err.Error(), nil)
		return
	}
	l.Debug("GC user approvals", nil)
}

func (s *service) UserResetGCHook(ctx context.Context, topic string, msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": s.syschannels.GC,
		"group":   s.streamns + "_WORKER_RESET_GC",
	})
	props, err := governor.DecodeSysEventTimestampProps(msgdata)
	if err != nil {
		l.Error(err.Error(), nil)
		return
	}
	if err := s.resets.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		l.Error(err.Error(), nil)
		return
	}
	l.Debug("GC user resets", nil)
}

func (s *service) UserInvitationGCHook(ctx context.Context, topic string, msgdata []byte) {
	l := s.logger.WithData(map[string]string{
		"agent":   "subscriber",
		"channel": s.syschannels.GC,
		"group":   s.streamns + "_WORKER_INVITATION_GC",
	})
	props, err := governor.DecodeSysEventTimestampProps(msgdata)
	if err != nil {
		l.Error(err.Error(), nil)
		return
	}
	if err := s.invitations.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		l.Error(err.Error(), nil)
		return
	}
	l.Debug("GC user invitations", nil)
}
