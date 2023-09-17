package user

import (
	"context"
	"encoding/json"
	htmlTemplate "html/template"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/events/sysevent"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/service/gate/apikey"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/approvalmodel"
	"xorkevin.dev/governor/service/user/resetmodel"
	"xorkevin.dev/governor/service/user/sessionmodel"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/hunter2/h2cipher"
	"xorkevin.dev/hunter2/h2cipher/aes"
	"xorkevin.dev/hunter2/h2cipher/xchacha20poly1305"
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
		StreamName() string
	}

	otpCipher struct {
		cipher  h2cipher.Cipher
		keyring *h2cipher.Keyring
	}

	eventSettings struct {
		streamNS    string
		streamUsers string
		streamSize  int64
		eventSize   int64
	}

	authSettings struct {
		accessDuration  time.Duration
		refreshDuration time.Duration
		newLoginEmail   bool
		passMinSize     int
		otpIssuer       string
		sudoDuration    time.Duration
	}

	editSettings struct {
		newUserApproval         bool
		newUserConfirmDuration  time.Duration
		newEmailConfirmDuration time.Duration
		passReset               bool
		passResetDuration       time.Duration
		passResetDelay          time.Duration
	}

	emailSettings struct {
		tplName emailTplName
		urlTpl  emailURLTpl
	}

	emailTplName struct {
		newuser           string
		emailchange       string
		emailchangenotify string
		passchange        string
		forgotpass        string
		passreset         string
		loginratelimit    string
		otpbackupused     string
	}

	emailURLTpl struct {
		base        string
		newUser     *htmlTemplate.Template
		emailChange *htmlTemplate.Template
		forgotPass  *htmlTemplate.Template
	}

	Service struct {
		users         usermodel.Repo
		sessions      sessionmodel.Repo
		approvals     approvalmodel.Repo
		resets        resetmodel.Repo
		acl           authzacl.Manager
		apikeys       apikey.Apikeys
		kvotpcodes    kvstore.KVStore
		pubsub        pubsub.Pubsub
		events        events.Events
		mailer        mail.Mailer
		ratelimiter   ratelimit.Ratelimiter
		gate          gate.Manager
		lc            *lifecycle.Lifecycle[otpCipher]
		cipherAlgs    h2cipher.Algs
		config        governor.ConfigReader
		log           *klog.LevelLogger
		tracer        governor.Tracer
		rolens        string
		scopens       string
		eventSettings eventSettings
		baseURL       string
		authURL       string
		authSettings  authSettings
		editSettings  editSettings
		emailSettings emailSettings
		hbfailed      int
		hbmaxfail     int
		otprefresh    time.Duration
		gcDuration    time.Duration
		wg            *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}
)

// New creates a new Users service
func New(
	users usermodel.Repo,
	sessions sessionmodel.Repo,
	approvals approvalmodel.Repo,
	resets resetmodel.Repo,
	acl authzacl.Manager,
	apikeys apikey.Apikeys,
	kv kvstore.KVStore,
	ps pubsub.Pubsub,
	ev events.Events,
	mailer mail.Mailer,
	ratelimiter ratelimit.Ratelimiter,
	g gate.Manager,
) *Service {
	cipherAlgs := h2cipher.NewAlgsMap()
	xchacha20poly1305.Register(cipherAlgs)
	aes.Register(cipherAlgs)
	return &Service{
		users:       users,
		sessions:    sessions,
		approvals:   approvals,
		resets:      resets,
		acl:         acl,
		apikeys:     apikeys,
		kvotpcodes:  kv.Subtree("otpcodes"),
		pubsub:      ps,
		events:      ev,
		mailer:      mailer,
		ratelimiter: ratelimiter,
		gate:        g,
		cipherAlgs:  cipherAlgs,
		hbfailed:    0,
		wg:          ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	s.rolens = "gov." + r.Name()
	s.scopens = "gov." + r.Name()
	s.eventSettings = eventSettings{
		streamNS:    r.Name(),
		streamUsers: r.Name(),
	}

	r.SetDefault("event.streamSize", "200M")
	r.SetDefault("event.eventSize", "2K")

	r.SetDefault("auth.accessDuration", "5m")
	r.SetDefault("auth.refreshDuration", "4380h")
	r.SetDefault("auth.newLoginEmail", true)
	r.SetDefault("auth.passMinSize", 8)
	r.SetDefault("auth.otpIssuer", "governor")
	r.SetDefault("auth.sudoDuration", "1h")

	r.SetDefault("edit.newUserApproval", false)
	r.SetDefault("edit.newUserConfirmDuration", "24h")
	r.SetDefault("edit.newEmailConfirmDuration", "24h")
	r.SetDefault("edit.passReset", true)
	r.SetDefault("edit.passResetDuration", "24h")
	r.SetDefault("edit.passResetDelay", "1h")

	r.SetDefault("email.tpl.emailchange", "emailchange")
	r.SetDefault("email.tpl.emailchangenotify", "emailchangenotify")
	r.SetDefault("email.tpl.passchange", "passchange")
	r.SetDefault("email.tpl.forgotpass", "forgotpass")
	r.SetDefault("email.tpl.passreset", "passreset")
	r.SetDefault("email.tpl.loginratelimit", "loginratelimit")
	r.SetDefault("email.tpl.otpbackupused", "otpbackupused")
	r.SetDefault("email.tpl.newuser", "newuser")

	r.SetDefault("email.url.base", "http://localhost:8080")
	r.SetDefault("email.url.newUser", "/x/confirm?userid={{.Userid}}&key={{.Key}}")
	r.SetDefault("email.url.emailChange", "/a/confirm/email?key={{.Userid}}.{{.Key}}")
	r.SetDefault("email.url.forgotPass", "/x/resetpass?key={{.Userid}}.{{.Key}}")

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

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)
	s.tracer = kit.Tracer
	s.config = r

	var err error
	s.eventSettings.streamSize, err = bytefmt.ToBytes(r.GetStr("event.streamSize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid event stream size")
	}
	s.eventSettings.eventSize, err = bytefmt.ToBytes(r.GetStr("event.eventSize"))
	if err != nil {
		return kerrors.WithMsg(err, "Invalid event msg size")
	}

	s.baseURL = r.Config().BasePath
	s.authURL = s.baseURL + r.URL() + authRoutePrefix

	s.authSettings.accessDuration, err = r.GetDuration("auth.accessDuration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse access duration")
	}
	s.authSettings.refreshDuration, err = r.GetDuration("auth.refreshDuration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse refresh duration")
	}
	s.authSettings.newLoginEmail = r.GetBool("auth.newLoginEmail")
	s.authSettings.passMinSize = r.GetInt("auth.passMinSize")
	s.authSettings.otpIssuer = r.GetStr("auth.otpIssuer")
	s.authSettings.sudoDuration, err = r.GetDuration("auth.sudoDuration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse sudo duration")
	}

	s.editSettings.newUserApproval = r.GetBool("edit.newUserApproval")
	s.editSettings.newUserConfirmDuration, err = r.GetDuration("edit.newUserConfirmDuration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse new user confirm duration")
	}
	s.editSettings.newEmailConfirmDuration, err = r.GetDuration("edit.newEmailConfirmDuration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse new email confirm duration")
	}
	s.editSettings.passReset = r.GetBool("edit.passReset")
	s.editSettings.passResetDuration, err = r.GetDuration("edit.passResetDuration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse password reset duration")
	}
	s.editSettings.passResetDelay, err = r.GetDuration("edit.passResetDelay")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse password reset delay")
	}

	s.emailSettings = emailSettings{
		tplName: emailTplName{
			newuser:           r.GetStr("email.tpl.newuser"),
			emailchange:       r.GetStr("email.tpl.emailchange"),
			emailchangenotify: r.GetStr("email.tpl.emailchangenotify"),
			passchange:        r.GetStr("email.tpl.passchange"),
			forgotpass:        r.GetStr("email.tpl.forgotpass"),
			passreset:         r.GetStr("email.tpl.passreset"),
			loginratelimit:    r.GetStr("email.tpl.loginratelimit"),
			otpbackupused:     r.GetStr("email.tpl.otpbackupused"),
		},
	}

	s.emailSettings.urlTpl.base = r.GetStr("email.url.base")
	if t, err := htmlTemplate.New("email.url.newUser").Parse(r.GetStr("email.url.newUser")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse new user url template")
	} else {
		s.emailSettings.urlTpl.newUser = t
	}
	if t, err := htmlTemplate.New("email.url.emailChange").Parse(r.GetStr("email.url.emailChange")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse email change url template")
	} else {
		s.emailSettings.urlTpl.emailChange = t
	}
	if t, err := htmlTemplate.New("email.url.forgotPass").Parse(r.GetStr("email.url.forgotPass")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse forgot pass url template")
	} else {
		s.emailSettings.urlTpl.forgotPass = t
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

	s.log.Info(ctx, "Loaded config",
		klog.AString("event.streamSize", bytefmt.ToString(s.eventSettings.streamSize)),
		klog.AString("event.eventSize", bytefmt.ToString(s.eventSettings.eventSize)),

		klog.AString("auth.accessDuration", s.authSettings.accessDuration.String()),
		klog.AString("auth.refreshDuration", s.authSettings.refreshDuration.String()),
		klog.ABool("auth.newLoginEmail", s.authSettings.newLoginEmail),
		klog.AInt("auth.passMinSize", s.authSettings.passMinSize),
		klog.AString("auth.otpIssuer", s.authSettings.otpIssuer),
		klog.AString("auth.sudoDuration", s.authSettings.sudoDuration.String()),

		klog.ABool("edit.newUserApproval", s.editSettings.newUserApproval),
		klog.AString("edit.newUserConfirmDuration", s.editSettings.newUserConfirmDuration.String()),
		klog.AString("edit.newEmailConfirmDuration", s.editSettings.newEmailConfirmDuration.String()),
		klog.ABool("edit.passReset", s.editSettings.passReset),
		klog.AString("edit.passResetDuration", s.editSettings.passResetDuration.String()),
		klog.AString("edit.passResetDelay", s.editSettings.passResetDelay.String()),

		klog.AString("email.tpl.newuser", s.emailSettings.tplName.newuser),
		klog.AString("email.tpl.emailchange", s.emailSettings.tplName.emailchange),
		klog.AString("email.tpl.emailchangenotify", s.emailSettings.tplName.emailchangenotify),
		klog.AString("email.tpl.passchange", s.emailSettings.tplName.passchange),
		klog.AString("email.tpl.forgotpass", s.emailSettings.tplName.forgotpass),
		klog.AString("email.tpl.passreset", s.emailSettings.tplName.passreset),
		klog.AString("email.tpl.loginratelimit", s.emailSettings.tplName.loginratelimit),
		klog.AString("email.tpl.otpbackupused", s.emailSettings.tplName.otpbackupused),

		klog.AString("email.url.base", s.emailSettings.urlTpl.base),
		klog.AString("email.url.newUser", r.GetStr("email.url.newUser")),
		klog.AString("email.url.emailChange", r.GetStr("email.url.emailChange")),
		klog.AString("email.url.forgotPass", r.GetStr("email.url.forgotPass")),

		klog.AString("hbinterval", hbinterval.String()),
		klog.AInt("hbmaxfail", s.hbmaxfail),
		klog.AString("otprefresh", s.otprefresh.String()),
		klog.AString("gcduration", s.gcDuration.String()),
	)

	sr := s.router()
	sr.mountAuth(kit.Router.GroupCtx(authRoutePrefix, sr.rt))
	sr.mountRoute(kit.Router.GroupCtx("/user"))
	sr.mountApikey(kit.Router.GroupCtx("/apikey"))
	s.log.Info(ctx, "Mounted http routes")

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "run"))
	s.lc = lifecycle.New(
		ctx,
		s.handleGetCipher,
		s.closeCipher,
		s.handlePing,
		hbinterval,
	)
	go s.lc.Heartbeat(ctx, s.wg)

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
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to refresh otp secrets"))
		return
	}
	s.log.Err(ctx, kerrors.WithMsg(err, "Failed max refresh attempts"))
	s.hbfailed = 0
	// clear the cached cipher because its secret may be invalid
	m.Stop(ctx)
}

func (s *Service) handleGetCipher(ctx context.Context, m *lifecycle.State[otpCipher]) (*otpCipher, error) {
	currentCipher := m.Load(ctx)
	var otpsecrets secretOTP
	if err := s.config.GetSecret(ctx, "otpkey", s.otprefresh, &otpsecrets); err != nil {
		return nil, kerrors.WithMsg(err, "Invalid otpkey secrets")
	}
	if len(otpsecrets.Keys) == 0 {
		return nil, kerrors.WithKind(nil, governor.ErrInvalidConfig, "No otpkey present")
	}
	keyring := h2cipher.NewKeyring()
	var hcipher h2cipher.Cipher
	for n, i := range otpsecrets.Keys {
		c, err := h2cipher.FromParams(i, s.cipherAlgs)
		if err != nil {
			return nil, kerrors.WithKind(err, governor.ErrInvalidConfig, "Invalid cipher param")
		}
		if n == 0 {
			if currentCipher != nil && currentCipher.cipher.ID() == c.ID() {
				// first, newest cipher matches current cipher, therefore no change in ciphers
				return currentCipher, nil
			}
			hcipher = c
		}
		keyring.Register(c)
	}

	m.Stop(ctx)

	cipher := &otpCipher{
		cipher:  hcipher,
		keyring: keyring,
	}

	s.log.Info(ctx, "Refreshed otp secrets with new secrets",
		klog.AString("kid", cipher.cipher.ID()),
		klog.AInt("numkeys", keyring.Size()),
	)

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

func (s *Service) Start(ctx context.Context) error {
	userEventWatcher := events.NewWatcher(
		s.events,
		s.log.Logger,
		s.tracer,
		s.eventSettings.streamUsers,
		s.eventSettings.streamNS+".worker",
		events.ConsumerOpts{},
		events.HandlerFunc(s.userEventHandler),
		nil,
		0,
	)
	s.wg.Add(1)
	go userEventWatcher.Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream")

	gcWatcher := pubsub.NewWatcher(
		s.pubsub,
		s.log.Logger,
		s.tracer,
		sysevent.GCChannel,
		s.eventSettings.streamNS+".worker.gc",
		pubsub.HandlerFunc(s.userEventHandlerGC),
	)
	s.wg.Add(1)
	go gcWatcher.Watch(ctx, s.wg, pubsub.WatchOpts{})
	s.log.Info(ctx, "Subscribed to gov sys gc channel")

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.events.InitStream(ctx, s.eventSettings.streamUsers, events.StreamOpts{
		Partitions:     16,
		Replicas:       1,
		ReplicaQuorum:  1,
		RetentionAge:   30 * 24 * time.Hour,
		RetentionBytes: int(s.eventSettings.streamSize),
		MaxMsgBytes:    int(s.eventSettings.eventSize),
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to init user stream")
	}
	s.log.Info(ctx, "Created user stream")

	if err := s.users.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created user table")

	if err := s.sessions.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created usersessions table")

	if err := s.approvals.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userapprovals table")

	if err := s.resets.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created userresets table")

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	if s.lc.Load(ctx) == nil {
		return kerrors.WithKind(nil, governor.ErrInvalidConfig, "User service not ready")
	}
	return nil
}

func (s *Service) StreamName() string {
	return s.eventSettings.streamUsers
}

// ErrUserEvent is returned when the user event is malformed
var ErrUserEvent errUserEvent

type (
	errUserEvent struct{}
)

func (e errUserEvent) Error() string {
	return "Malformed user event"
}

// DecodeUserEvent decodes a user event
func DecodeUserEvent(msgdata []byte) (*UserEvent, error) {
	var m userEventDec
	if err := kjson.Unmarshal(msgdata, &m); err != nil {
		return nil, kerrors.WithKind(err, ErrUserEvent, "Failed to decode user event")
	}
	props := &UserEvent{
		Kind: m.Kind,
	}
	switch m.Kind {
	case UserEventKindCreate:
		if err := kjson.Unmarshal(m.Payload, &props.Create); err != nil {
			return nil, kerrors.WithKind(err, ErrUserEvent, "Failed to decode create user event")
		}
	case UserEventKindUpdate:
		if err := kjson.Unmarshal(m.Payload, &props.Update); err != nil {
			return nil, kerrors.WithKind(err, ErrUserEvent, "Failed to decode update user event")
		}
	case UserEventKindDelete:
		if err := kjson.Unmarshal(m.Payload, &props.Delete); err != nil {
			return nil, kerrors.WithKind(err, ErrUserEvent, "Failed to decode delete user event")
		}
	default:
		return nil, kerrors.WithKind(nil, ErrUserEvent, "Invalid user event kind")
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

const (
	roleDeleteBatchSize   = 256
	apikeyDeleteBatchSize = 256
)

func (s *Service) userEventHandler(ctx context.Context, msg events.Msg) error {
	props, err := DecodeUserEvent(msg.Value)
	if err != nil {
		return err
	}
	switch props.Kind {
	case UserEventKindDelete:
		return s.userEventHandlerDelete(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userEventHandlerDelete(ctx context.Context, props DeleteUserProps) error {
	if err := s.resets.DeleteByUserid(ctx, props.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user resets")
	}
	if err := s.killAllSessions(ctx, props.Userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user sessions")
	}
	for {
		r, err := s.acl.ReadBySub(ctx, authzacl.Sub{NS: gate.NSUser, Key: props.Userid}, roleDeleteBatchSize, nil)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get user acl tuples")
		}
		if len(r) == 0 {
			break
		}
		rels := make([]authzacl.Relation, 0, len(r))
		for _, i := range r {
			rels = append(rels, authzacl.Relation{
				Obj: i,
				Sub: authzacl.Sub{NS: gate.NSUser, Key: props.Userid},
			})
		}
		if err := s.acl.DeleteRelations(ctx, rels); err != nil {
			return kerrors.WithMsg(err, "Failed to delete user acl tuples")
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

func (s *Service) userEventHandlerGC(ctx context.Context, m pubsub.Msg) error {
	props, err := sysevent.DecodeGCEvent(m.Data)
	if err != nil {
		return err
	}
	if err := s.approvals.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to GC approvals"))
	} else {
		s.log.Info(ctx, "GC user approvals")
	}
	if err := s.resets.DeleteBefore(ctx, time.Unix(props.Timestamp, 0).Add(-s.gcDuration).Unix()); err != nil {
		s.log.Err(ctx, kerrors.WithMsg(err, "Failed to GC resets"))
	} else {
		s.log.Info(ctx, "GC user resets")
	}
	return nil
}
