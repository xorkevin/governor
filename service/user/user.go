package user

import (
	"context"
	"encoding/json"
	htmlTemplate "html/template"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/events/sysevent"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/pubsub"
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
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/hunter2"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	authRoutePrefix = "/auth"
)

// User event kinds
const (
	UserEventKindCreate = "create"
	UserEventKindUpdate = "update"
	UserEventKindDelete = "delete"
	UserEventKindRoles  = "roles"
)

type (
	userEventDec struct {
		Kind    string          `json:"kind"`
		Payload json.RawMessage `json:"payload"`
	}

	userEventEnc struct {
		Kind    string      `json:"kind"`
		Payload interface{} `json:"payload"`
	}

	// UserEvent is a user event
	UserEvent struct {
		Kind   string
		Create CreateUserProps
		Update UpdateUserProps
		Delete DeleteUserProps
		Roles  RolesProps
	}

	// CreateUserProps are properties of a newly created user
	CreateUserProps struct {
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

	// RolesProps are properties of a user role update
	RolesProps struct {
		Add    bool     `json:"add"`
		Userid string   `json:"userid"`
		Roles  []string `json:"roles"`
	}

	// HandlerFunc is a user event handler
	HandlerFunc = func(ctx context.Context, props UserEvent) error

	// Users is a user management service
	Users interface {
		GetByID(ctx context.Context, userid string) (*ResUserGet, error)
		GetByUsername(ctx context.Context, username string) (*ResUserGet, error)
		GetByEmail(ctx context.Context, email string) (*ResUserGet, error)
		GetInfoBulk(ctx context.Context, userids []string) (*ResUserInfoList, error)
		CheckUserExists(ctx context.Context, userid string) (bool, error)
		CheckUsersExist(ctx context.Context, userids []string) ([]string, error)
		DeleteRoleInvitations(ctx context.Context, role string) error
		GetRoleUsers(ctx context.Context, roleName string, amount, offset int) ([]string, error)
		InsertRoles(ctx context.Context, userid string, roles rank.Rank) error
		DeleteRolesByRole(ctx context.Context, roleName string, userids []string) error
		WatchUsers(group string, opts events.ConsumerOpts, handler, dlqhandler HandlerFunc, maxdeliver int) *events.Watcher
	}

	otpCipher struct {
		cipher    hunter2.Cipher
		decrypter *hunter2.Decrypter
	}

	authSettings struct {
		accessDuration        time.Duration
		refreshDuration       time.Duration
		refreshCache          time.Duration
		confirmDuration       time.Duration
		emailConfirmDuration  time.Duration
		passwordReset         bool
		passwordResetDuration time.Duration
		passResetDelay        time.Duration
		invitationDuration    time.Duration
		userCacheDuration     time.Duration
		newLoginEmail         bool
		passwordMinSize       int
		userApproval          bool
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

	Service struct {
		users        model.Repo
		sessions     sessionmodel.Repo
		approvals    approvalmodel.Repo
		invitations  invitationmodel.Repo
		resets       resetmodel.Repo
		roles        role.RolesManager
		apikeys      apikey.Apikeys
		kvusers      kvstore.KVStore
		kvsessions   kvstore.KVStore
		kvotpcodes   kvstore.KVStore
		pubsub       pubsub.Pubsub
		events       events.Events
		mailer       mail.Mailer
		ratelimiter  ratelimit.Ratelimiter
		gate         gate.Gate
		tokenizer    token.Tokenizer
		lc           *lifecycle.Lifecycle[otpCipher]
		syschannels  governor.SysChannels
		instance     string
		config       governor.SecretReader
		log          *klog.LevelLogger
		rolens       string
		scopens      string
		streamns     string
		streamusers  string
		streamsize   int64
		eventsize    int32
		baseURL      string
		authURL      string
		authsettings authSettings
		otpIssuer    string
		rolesummary  rank.Rank
		tplname      tplName
		emailurl     emailURLTpl
		hbfailed     int
		hbmaxfail    int
		otprefresh   time.Duration
		gcDuration   time.Duration
		wg           *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}

	ctxKeyUsers struct{}
)

// GetCtxUsers returns a [Users] service from the context
func GetCtxUsers(inj governor.Injector) Users {
	v := inj.Get(ctxKeyUsers{})
	if v == nil {
		return nil
	}
	return v.(Users)
}

// setCtxUser sets a [Users] service in the context
func setCtxUser(inj governor.Injector, u Users) {
	inj.Set(ctxKeyUsers{}, u)
}

// NewCtx creates a new [Users] service from a context
func NewCtx(inj governor.Injector) *Service {
	return New(
		model.GetCtxRepo(inj),
		sessionmodel.GetCtxRepo(inj),
		approvalmodel.GetCtxRepo(inj),
		invitationmodel.GetCtxRepo(inj),
		resetmodel.GetCtxRepo(inj),
		role.GetCtxRolesManager(inj),
		apikey.GetCtxApikeys(inj),
		kvstore.GetCtxKVStore(inj),
		pubsub.GetCtxPubsub(inj),
		events.GetCtxEvents(inj),
		mail.GetCtxMailer(inj),
		ratelimit.GetCtxRatelimiter(inj),
		token.GetCtxTokenizer(inj),
		gate.GetCtxGate(inj),
	)
}

// New creates a new Users service
func New(
	users model.Repo,
	sessions sessionmodel.Repo,
	approvals approvalmodel.Repo,
	invitations invitationmodel.Repo,
	resets resetmodel.Repo,
	roles role.RolesManager,
	apikeys apikey.Apikeys,
	kv kvstore.KVStore,
	ps pubsub.Pubsub,
	ev events.Events,
	mailer mail.Mailer,
	ratelimiter ratelimit.Ratelimiter,
	tokenizer token.Tokenizer,
	g gate.Gate,
) *Service {
	return &Service{
		users:       users,
		sessions:    sessions,
		approvals:   approvals,
		invitations: invitations,
		resets:      resets,
		roles:       roles,
		apikeys:     apikeys,
		kvusers:     kv.Subtree("users"),
		kvsessions:  kv.Subtree("sessions"),
		kvotpcodes:  kv.Subtree("otpcodes"),
		pubsub:      ps,
		events:      ev,
		mailer:      mailer,
		ratelimiter: ratelimiter,
		gate:        g,
		tokenizer:   tokenizer,
		hbfailed:    0,
		wg:          ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxUser(inj, s)
	s.rolens = "gov." + r.Name()
	s.scopens = "gov." + r.Name()
	s.streamns = r.Name()
	s.streamusers = r.Name()

	r.SetDefault("streamsize", "200M")
	r.SetDefault("eventsize", "2K")
	r.SetDefault("accessduration", "5m")
	r.SetDefault("refreshduration", "4380h")
	r.SetDefault("refreshcache", "24h")
	r.SetDefault("confirmduration", "24h")
	r.SetDefault("emailconfirmduration", "24h")
	r.SetDefault("passwordreset", true)
	r.SetDefault("passwordresetduration", "24h")
	r.SetDefault("passresetdelay", "1h")
	r.SetDefault("invitationduration", "24h")
	r.SetDefault("usercacheduration", "24h")
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
	r.SetDefault("hbinterval", "5s")
	r.SetDefault("hbmaxfail", 6)
	r.SetDefault("otprefresh", "1m")
	r.SetDefault("gcduration", "72h")
}

func (s *Service) router() *router {
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

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)
	s.config = r
	s.instance = c.Instance
	s.syschannels = c.SysChannels

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
	s.authsettings.accessDuration, err = r.GetDuration("accessduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse access duration")
	}
	s.authsettings.refreshDuration, err = r.GetDuration("refreshduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse refresh duration")
	}
	s.authsettings.refreshCache, err = r.GetDuration("refreshcache")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse refresh cache duration")
	}
	s.authsettings.confirmDuration, err = r.GetDuration("confirmduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse confirm duration")
	}
	s.authsettings.emailConfirmDuration, err = r.GetDuration("emailconfirmduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse confirm duration")
	}
	s.authsettings.passwordReset = r.GetBool("passwordreset")
	s.authsettings.passwordResetDuration, err = r.GetDuration("passwordresetduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse password reset duration")
	}
	s.authsettings.passResetDelay, err = r.GetDuration("passresetdelay")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse password reset delay")
	}
	s.authsettings.invitationDuration, err = r.GetDuration("invitationduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse role invitation duration")
	}
	s.authsettings.userCacheDuration, err = r.GetDuration("usercacheduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse user cache duration")
	}
	s.authsettings.newLoginEmail = r.GetBool("newloginemail")
	s.authsettings.passwordMinSize = r.GetInt("passwordminsize")
	s.authsettings.userApproval = r.GetBool("userapproval")
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

	hbinterval, err := r.GetDuration("hbinterval")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse hbinterval")
	}
	s.hbmaxfail = r.GetInt("hbmaxfail")
	s.otprefresh, err = r.GetDuration("otprefresh")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse otprefresh")
	}
	s.gcDuration, err = r.GetDuration("gcduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse gcduration")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"user.stream.size":               r.GetStr("streamsize"),
		"user.event.size":                r.GetStr("eventsize"),
		"user.auth.accessduration":       s.authsettings.accessDuration.String(),
		"user.auth.refreshduration":      s.authsettings.refreshDuration.String(),
		"user.auth.refreshcache":         s.authsettings.refreshCache.String(),
		"user.confirmduration":           s.authsettings.confirmDuration.String(),
		"user.emailconfirmduration":      s.authsettings.emailConfirmDuration.String(),
		"user.passwordresetallowed":      s.authsettings.passwordReset,
		"user.passwordresetduration":     s.authsettings.passwordResetDuration.String(),
		"user.passresetdelay":            s.authsettings.passResetDelay.String(),
		"user.invitationduration":        s.authsettings.invitationDuration.String(),
		"user.cacheduration":             s.authsettings.userCacheDuration.String(),
		"user.newlogin.email":            s.authsettings.newLoginEmail,
		"user.passwordminsize":           s.authsettings.passwordMinSize,
		"user.approvalrequired":          s.authsettings.userApproval,
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
		"user.hbinterval":                hbinterval.String(),
		"user.hbmaxfail":                 s.hbmaxfail,
		"user.otprefresh":                s.otprefresh.String(),
		"user.gcduration":                s.gcDuration.String(),
	})

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "run",
	})

	s.lc = lifecycle.New(
		ctx,
		s.handleGetCipher,
		s.closeCipher,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

	sr := s.router()
	sr.mountRoute(m.GroupCtx("/user"))
	sr.mountAuth(m.GroupCtx(authRoutePrefix, sr.rt))
	sr.mountApikey(m.GroupCtx("/apikey"))
	s.log.Info(ctx, "Mounted http routes", nil)
	return nil
}

func (s *Service) handlePing(ctx context.Context, m *lifecycle.Manager[otpCipher]) {
	_, err := m.Construct(ctx)
	if err == nil {
		s.hbfailed = 0
		return
	}
	s.hbfailed++
	if s.hbfailed < s.hbmaxfail {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh otp secrets"), nil)
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"), nil)
	s.hbfailed = 0
	// clear the cached cipher because its secret may be invalid
	m.Stop(ctx)
}

func (s *Service) handleGetCipher(ctx context.Context, m *lifecycle.Manager[otpCipher]) (*otpCipher, error) {
	currentCipher := m.Load(ctx)
	var otpsecrets secretOTP
	if err := s.config.GetSecret(ctx, "otpkey", s.otprefresh, &otpsecrets); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid otpkey secrets")
	}
	if len(otpsecrets.Keys) == 0 {
		return nil, kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "No otpkey present")
	}
	decrypter := hunter2.NewDecrypter()
	var hcipher hunter2.Cipher
	for n, i := range otpsecrets.Keys {
		c, err := hunter2.CipherFromParams(i, hunter2.DefaultCipherAlgs)
		if err != nil {
			return nil, kerrors.WithKind(err, governor.ErrorInvalidConfig{}, "Invalid cipher param")
		}
		if n == 0 {
			if currentCipher != nil && currentCipher.cipher.ID() == c.ID() {
				// first, newest cipher matches current cipher, therefore no change in ciphers
				return currentCipher, nil
			}
			hcipher = c
		}
		decrypter.RegisterCipher(c)
	}

	m.Stop(ctx)

	cipher := &otpCipher{
		cipher:    hcipher,
		decrypter: decrypter,
	}

	s.log.Info(ctx, "Refreshed otp secrets with new secrets", klog.Fields{
		"user.otpcipher.kid":        cipher.cipher.ID(),
		"user.otpcipher.numotpkeys": decrypter.Size(),
	})

	m.Store(cipher)

	return cipher, nil
}

func (s *Service) closeCipher(ctx context.Context, cipher *otpCipher) {
	// nothing to close
}

func (s *Service) getCipher(ctx context.Context) (*otpCipher, error) {
	if cipher := s.lc.Load(ctx); cipher != nil {
		return cipher, nil
	}

	return s.lc.Construct(ctx)
}

func (s *Service) addAdmin(ctx context.Context, req reqAddAdmin) error {
	madmin, err := s.users.New(req.Username, req.Password, req.Email, req.Firstname, req.Lastname)
	if err != nil {
		return err
	}

	b0, err := encodeUserEventCreate(CreateUserProps{
		Userid:       madmin.Userid,
		Username:     madmin.Username,
		Email:        madmin.Email,
		FirstName:    madmin.FirstName,
		LastName:     madmin.LastName,
		CreationTime: madmin.CreationTime,
	})
	if err != nil {
		return err
	}
	b1, err := encodeUserEventRoles(RolesProps{
		Add:    true,
		Userid: madmin.Userid,
		Roles:  rank.Admin().ToSlice(),
	})
	if err != nil {
		return err
	}

	if err := s.users.Insert(ctx, madmin); err != nil {
		return err
	}

	// must make best effort to add roles, publish new user event, and clear user existence cache
	ctx = klog.ExtendCtx(context.Background(), ctx, nil)

	if err := s.roles.InsertRoles(ctx, madmin.Userid, rank.Admin()); err != nil {
		return kerrors.WithMsg(err, "Failed to add user roles")
	}

	if err := s.events.Publish(ctx, events.NewMsgs(s.streamusers, madmin.Userid, b0, b1)...); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish create user event"), nil)
	}

	s.log.Info(ctx, "Added admin", klog.Fields{
		"user.username": madmin.Username,
		"user.userid":   madmin.Userid,
	})

	s.clearUserExists(ctx, madmin.Userid)

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.WatchUsers(s.streamns+".worker", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream", nil)

	sysEvents := sysevent.New(s.syschannels, s.pubsub, s.log.Logger)
	s.wg.Add(1)
	go sysEvents.WatchGC(s.streamns+".worker.gc", s.userEventHandlerGC, s.instance).Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to gov sys gc channel", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.streamusers, events.StreamOpts{
		Partitions:     16,
		Replicas:       1,
		ReplicaQuorum:  1,
		RetentionAge:   30 * 24 * time.Hour,
		RetentionBytes: int(s.streamsize),
		MaxMsgBytes:    int(s.eventsize),
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

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, governor.ErrorInvalidConfig{}, "User service not ready")
	}
	return nil
}

type (
	// ErrorUserEvent is returned when the user event is malformed
	ErrorUserEvent struct{}
)

func (e ErrorUserEvent) Error() string {
	return "Malformed user event"
}

func decodeUserEvent(msgdata []byte) (*UserEvent, error) {
	var m userEventDec
	if err := kjson.Unmarshal(msgdata, &m); err != nil {
		return nil, kerrors.WithKind(err, ErrorUserEvent{}, "Failed to decode user event")
	}
	props := &UserEvent{
		Kind: m.Kind,
	}
	switch m.Kind {
	case UserEventKindCreate:
		if err := kjson.Unmarshal(m.Payload, &props.Create); err != nil {
			return nil, kerrors.WithKind(err, ErrorUserEvent{}, "Failed to decode create user event")
		}
	case UserEventKindUpdate:
		if err := kjson.Unmarshal(m.Payload, &props.Update); err != nil {
			return nil, kerrors.WithKind(err, ErrorUserEvent{}, "Failed to decode update user event")
		}
	case UserEventKindDelete:
		if err := kjson.Unmarshal(m.Payload, &props.Delete); err != nil {
			return nil, kerrors.WithKind(err, ErrorUserEvent{}, "Failed to decode delete user event")
		}
	case UserEventKindRoles:
		if err := kjson.Unmarshal(m.Payload, &props.Roles); err != nil {
			return nil, kerrors.WithKind(err, ErrorUserEvent{}, "Failed to decode roles user event")
		}
	default:
		return nil, kerrors.WithKind(nil, ErrorUserEvent{}, "Invalid user event kind")
	}
	return props, nil
}

func encodeUserEventCreate(props CreateUserProps) ([]byte, error) {
	b, err := kjson.Marshal(userEventEnc{
		Kind:    UserEventKindCreate,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode create user props to json")
	}
	return b, nil
}

func encodeUserEventUpdate(props UpdateUserProps) ([]byte, error) {
	b, err := kjson.Marshal(userEventEnc{
		Kind:    UserEventKindUpdate,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode update user props to json")
	}
	return b, nil
}

func encodeUserEventDelete(props DeleteUserProps) ([]byte, error) {
	b, err := kjson.Marshal(userEventEnc{
		Kind:    UserEventKindDelete,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode delete user props to json")
	}
	return b, nil
}

func encodeUserEventRoles(props RolesProps) ([]byte, error) {
	b, err := kjson.Marshal(userEventEnc{
		Kind:    UserEventKindRoles,
		Payload: props,
	})
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to encode user roles props to json")
	}
	return b, nil
}

func (s *Service) WatchUsers(group string, opts events.ConsumerOpts, handler, dlqhandler HandlerFunc, maxdeliver int) *events.Watcher {
	var dlqfn events.Handler
	if dlqhandler != nil {
		dlqfn = events.HandlerFunc(func(ctx context.Context, msg events.Msg) error {
			props, err := decodeUserEvent(msg.Value)
			if err != nil {
				return err
			}
			return dlqhandler(ctx, *props)
		})
	}
	return events.NewWatcher(s.events, s.log.Logger, s.streamusers, group, opts, events.HandlerFunc(func(ctx context.Context, msg events.Msg) error {
		props, err := decodeUserEvent(msg.Value)
		if err != nil {
			return err
		}
		return handler(ctx, *props)
	}), dlqfn, maxdeliver, s.instance)
}

const (
	roleDeleteBatchSize   = 256
	apikeyDeleteBatchSize = 256
)

func (s *Service) userEventHandler(ctx context.Context, props UserEvent) error {
	switch props.Kind {
	case UserEventKindDelete:
		return s.userEventHandlerDelete(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userEventHandlerDelete(ctx context.Context, props DeleteUserProps) error {
	for {
		r, err := s.roles.GetRoles(ctx, props.Userid, "", roleDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user roles")
		}
		if len(r) == 0 {
			break
		}
		b, err := encodeUserEventRoles(RolesProps{
			Add:    false,
			Userid: props.Userid,
			Roles:  r.ToSlice(),
		})
		if err != nil {
			return err
		}
		if err := s.events.Publish(ctx, events.NewMsgs(s.streamusers, props.Userid, b)...); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Failed to publish delete roles event"), nil)
		}
		if err := s.roles.DeleteRoles(ctx, props.Userid, r); err != nil {
			return kerrors.WithMsg(err, "Failed to delete user roles")
		}
		if len(r) < roleDeleteBatchSize {
			break
		}
	}
	for {
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

func (s *Service) userEventHandlerGC(ctx context.Context, props sysevent.TimestampProps) error {
	if err := s.approvals.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to GC approvals"), nil)
	} else {
		s.log.Info(ctx, "GC user approvals", nil)
	}
	if err := s.resets.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to GC resets"), nil)
	} else {
		s.log.Info(ctx, "GC user resets", nil)
	}
	if err := s.invitations.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to GC inviations"), nil)
	} else {
		s.log.Info(ctx, "GC user invitations", nil)
	}
	return nil
}
