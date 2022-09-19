package user

import (
	"context"
	htmlTemplate "html/template"
	"strings"
	"sync/atomic"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/events/sysevent"
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
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
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
	// WorkerFuncCreate is a type alias for a new user event consumer
	WorkerFuncCreate = func(ctx context.Context, pinger events.Pinger, props NewUserProps) error
	// WorkerFuncDelete is a type alias for a delete user event consumer
	WorkerFuncDelete = func(ctx context.Context, pinger events.Pinger, props DeleteUserProps) error
	// WorkerFuncUpdate is a type alias for an update user event consumer
	WorkerFuncUpdate = func(ctx context.Context, pinger events.Pinger, props UpdateUserProps) error

	// Users is a user management service
	Users interface {
		GetByID(ctx context.Context, userid string) (*ResUserGet, error)
		GetByUsername(ctx context.Context, username string) (*ResUserGet, error)
		GetByEmail(ctx context.Context, email string) (*ResUserGet, error)
		GetInfoBulk(ctx context.Context, userids []string) (*ResUserInfoList, error)
		CheckUserExists(ctx context.Context, userid string) (bool, error)
		CheckUsersExist(ctx context.Context, userids []string) ([]string, error)
		DeleteRoleInvitations(ctx context.Context, role string) error
		StreamSubscribeCreate(group string, worker WorkerFuncCreate, streamopts events.StreamConsumerOpts) (events.Subscription, error)
		StreamSubscribeDelete(group string, worker WorkerFuncDelete, streamopts events.StreamConsumerOpts) (events.Subscription, error)
		StreamSubscribeUpdate(group string, worker WorkerFuncUpdate, streamopts events.StreamConsumerOpts) (events.Subscription, error)
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
		log               *klog.LevelLogger
		rolens            string
		scopens           string
		streamns          string
		opts              svcOpts
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
		rt governor.MiddlewareCtx
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

	svcOpts struct {
		StreamName    string
		CreateChannel string
		DeleteChannel string
		UpdateChannel string
	}
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

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxUser(inj, s)
	s.rolens = "gov." + name
	s.scopens = "gov." + name
	streamname := strings.ToUpper(name)
	s.streamns = streamname
	s.opts = svcOpts{
		StreamName:    streamname,
		CreateChannel: streamname + ".create",
		DeleteChannel: streamname + ".delete",
		UpdateChannel: streamname + ".update",
	}

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
		rt: s.ratelimiter.BaseCtx(),
	}
}

type (
	secretOTP struct {
		Keys []string `mapstructure:"secrets"`
	}
)

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
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
	s.rolesummary, err = rank.FromSlice(r.GetStrSlice("rolesummary"))
	if err != nil {
		return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid rank for role summary")
	}

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

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"user.stream.size":               r.GetStr("streamsize"),
		"user.event.size":                r.GetStr("eventsize"),
		"user.auth.accesstime":           s.accessTime,
		"user.auth.refreshtime":          s.refreshTime,
		"user.auth.refreshcache":         s.refreshCacheTime,
		"user.confirmtime":               s.confirmTime,
		"user.passwordresetallowed":      s.passwordReset,
		"user.passwordresettime":         s.passwordResetTime,
		"user.passresetdelay":            s.passResetDelay,
		"user.invitationtime":            s.invitationTime,
		"user.cachetime":                 s.userCacheTime,
		"user.newlogin.email":            s.newLoginEmail,
		"user.passwordminsize":           s.passwordMinSize,
		"user.approvalrequired":          s.userApproval,
		"user.otpissuer":                 s.otpIssuer,
		"user.rolesummary":               s.rolesummary.String(),
		"user.tplname.emailchange":       s.tplname.emailchange,
		"user.tplname.emailchangenotify": s.tplname.emailchangenotify,
		"user.tplname.passchange":        s.tplname.passchange,
		"user.tplname.forgotpass":        s.tplname.forgotpass,
		"user.tplname.passreset":         s.tplname.passreset,
		"user.tplname.loginratelimit":    s.tplname.loginratelimit,
		"user.tplname.otpbackupused":     s.tplname.otpbackupused,
		"user.emailurlbase":              s.emailurl.base,
		"user.tpl.emailchange":           r.GetStr("email.url.emailchange"),
		"user.tpl.forgotpass":            r.GetStr("email.url.forgotpass"),
		"user.tpl.newuser":               r.GetStr("email.url.newuser"),
		"user.hbinterval":                s.hbinterval,
		"user.hbmaxfail":                 s.hbmaxfail,
		"user.otprefresh":                s.otprefresh,
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	done := make(chan struct{})
	go s.execute(ctx, done)
	s.done = done

	sr := s.router()
	sr.mountRoute(m.GroupCtx("/user"))
	sr.mountAuth(m.GroupCtx(authRoutePrefix, sr.rt))
	sr.mountApikey(m.GroupCtx("/apikey"))
	s.log.Info(ctx, "Mounted http routes", nil)
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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh otp secrets"), nil)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"), nil)
	s.ready.Store(false)
	s.hbfailed = 0
}

func (s *service) refreshSecrets(ctx context.Context) error {
	var otpsecrets secretOTP
	if err := s.config.GetSecret(ctx, "otpkey", int64(s.otprefresh), &otpsecrets); err != nil {
		return kerrors.WithMsg(err, "Invalid otpkey secrets")
	}
	if len(otpsecrets.Keys) == 0 {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "No otpkey present")
	}
	decrypter := hunter2.NewDecrypter()
	var cipher hunter2.Cipher
	for n, i := range otpsecrets.Keys {
		c, err := hunter2.CipherFromParams(i, hunter2.DefaultCipherAlgs)
		if err != nil {
			return kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid cipher param")
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
	s.log.Info(ctx, "Refreshed otp secrets with new secrets", klog.Fields{
		"user.otpcipher.kid":        s.otpCipher.cipher.ID(),
		"user.otpcipher.numotpkeys": decrypter.Size(),
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

func (s *service) AddAdmin(ctx context.Context, req governor.ReqAddAdmin) error {
	madmin, err := s.users.New(req.Username, req.Password, req.Email, req.Firstname, req.Lastname)
	if err != nil {
		return err
	}

	b, err := kjson.Marshal(NewUserProps{
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

	if err := s.users.Insert(ctx, madmin); err != nil {
		return err
	}

	// must make best effort to add roles and publish new user event
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)

	if err := s.roles.InsertRoles(ctx, madmin.Userid, rank.Admin()); err != nil {
		return err
	}

	if err := s.events.StreamPublish(ctx, s.opts.CreateChannel, b); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish new user"), nil)
	}

	s.log.Info(ctx, "Added admin", klog.Fields{
		"user.username": madmin.Username,
		"user.userid":   madmin.Userid,
	})

	return nil
}

func (s *service) Start(ctx context.Context) error {
	if _, err := s.StreamSubscribeDelete(s.streamns+"_WORKER_ROLE_DELETE", s.userRoleDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	s.log.Info(ctx, "Subscribed to user delete queue", nil)

	if _, err := s.StreamSubscribeDelete(s.streamns+"_WORKER_APIKEY_DELETE", s.userApikeyDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	s.log.Info(ctx, "Subscribed to user delete queue", nil)

	sysEvents := sysevent.New(s.syschannels, s.events)
	if _, err := sysEvents.SubscribeGC(s.streamns+"_WORKER_APPROVAL_GC", s.userApprovalGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	s.log.Info(ctx, "Subscribed to gov sys gc channel", nil)

	if _, err := sysEvents.SubscribeGC(s.streamns+"_WORKER_RESET_GC", s.userResetGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	s.log.Info(ctx, "Subscribed to gov sys gc channel", nil)

	if _, err := sysEvents.SubscribeGC(s.streamns+"_WORKER_INVITATION_GC", s.userInvitationGCHook); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to gov sys gc channel")
	}
	s.log.Info(ctx, "Subscribed to gov sys gc channel", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
	select {
	case <-s.done:
		return
	case <-ctx.Done():
		s.log.WarnErr(ctx, kerrors.WithMsg(ctx.Err(), "Failed to stop"), nil)
	}
}

func (s *service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.opts.StreamName, []string{s.opts.StreamName + ".>"}, events.StreamOpts{
		Replicas:   1,
		MaxAge:     30 * 24 * time.Hour,
		MaxBytes:   s.streamsize,
		MaxMsgSize: s.eventsize,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init user stream")
	}
	s.log.Info(ctx, "Created user stream", nil)

	if err := s.users.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created user table", nil)

	if err := s.sessions.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created usersessions table", nil)

	if err := s.approvals.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userapprovals table", nil)

	if err := s.invitations.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userroleinvitations table", nil)

	if err := s.resets.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userresets table", nil)

	return nil
}

func (s *service) Health(ctx context.Context) error {
	if !s.ready.Load() {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "User service not ready")
	}
	return nil
}

func decodeNewUserProps(msgdata []byte) (*NewUserProps, error) {
	m := &NewUserProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode new user props")
	}
	return m, nil
}

func decodeDeleteUserProps(msgdata []byte) (*DeleteUserProps, error) {
	m := &DeleteUserProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode delete user props")
	}
	return m, nil
}

func decodeUpdateUserProps(msgdata []byte) (*UpdateUserProps, error) {
	m := &UpdateUserProps{}
	if err := kjson.Unmarshal(msgdata, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to decode update user props")
	}
	return m, nil
}

func (s *service) StreamSubscribeCreate(group string, worker WorkerFuncCreate, streamopts events.StreamConsumerOpts) (events.Subscription, error) {
	sub, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.CreateChannel, group, func(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
		props, err := decodeNewUserProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, pinger, *props)
	}, streamopts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to user create channel")
	}
	return sub, nil
}

func (s *service) StreamSubscribeDelete(group string, worker WorkerFuncDelete, streamopts events.StreamConsumerOpts) (events.Subscription, error) {
	sub, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.DeleteChannel, group, func(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
		props, err := decodeDeleteUserProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, pinger, *props)
	}, streamopts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to user delete channel")
	}
	return sub, nil
}

func (s *service) StreamSubscribeUpdate(group string, worker WorkerFuncUpdate, streamopts events.StreamConsumerOpts) (events.Subscription, error) {
	sub, err := s.events.StreamSubscribe(s.opts.StreamName, s.opts.UpdateChannel, group, func(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
		props, err := decodeUpdateUserProps(msgdata)
		if err != nil {
			return err
		}
		return worker(ctx, pinger, *props)
	}, streamopts)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to subscribe to user update channel")
	}
	return sub, nil
}

const (
	roleDeleteBatchSize = 256
)

func (s *service) userRoleDeleteHook(ctx context.Context, pinger events.Pinger, props DeleteUserProps) error {
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

func (s *service) userApikeyDeleteHook(ctx context.Context, pinger events.Pinger, props DeleteUserProps) error {
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

func (s *service) userApprovalGCHook(ctx context.Context, props sysevent.TimestampProps) error {
	if err := s.approvals.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		return kerrors.WithMsg(err, "Failed to GC approvals")
	}
	s.log.Info(ctx, "GC user approvals", nil)
	return nil
}

func (s *service) userResetGCHook(ctx context.Context, props sysevent.TimestampProps) error {
	if err := s.resets.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		return kerrors.WithMsg(err, "Failed to GC resets")
	}
	s.log.Info(ctx, "GC user resets", nil)
	return nil
}

func (s *service) userInvitationGCHook(ctx context.Context, props sysevent.TimestampProps) error {
	if err := s.invitations.DeleteBefore(ctx, props.Timestamp-time72h); err != nil {
		return kerrors.WithMsg(err, "Failed to GC inviations")
	}
	s.log.Info(ctx, "GC user invitations", nil)
	return nil
}
