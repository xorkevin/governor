package oauth

import (
	"context"
	htmlTemplate "html/template"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/oauth/oauthappmodel"
	"xorkevin.dev/governor/service/user/oauth/oauthconnmodel"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	tokenRoute    = "/token"
	userinfoRoute = "/userinfo"
	jwksRoute     = "/jwks"
)

type (
	// OAuth manages OAuth apps
	OAuth interface{}

	Service struct {
		apps            oauthappmodel.Repo
		connections     oauthconnmodel.Repo
		tokenizer       token.Tokenizer
		kvclient        kvstore.KVStore
		oauthBucket     objstore.Bucket
		logoImgDir      objstore.Dir
		users           user.Users
		events          events.Events
		ratelimiter     ratelimit.Ratelimiter
		gate            gate.Gate
		log             *klog.LevelLogger
		rolens          string
		scopens         string
		streamns        string
		codeDuration    time.Duration
		accessDuration  time.Duration
		refreshDuration time.Duration
		keyCache        time.Duration
		realm           string
		issuer          string
		epauth          string
		eptoken         string
		epuserinfo      string
		epjwks          string
		tplprofile      *htmlTemplate.Template
		tplpicture      *htmlTemplate.Template
		wg              *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}

	ctxKeyOAuth struct{}
)

// GetCtxOAuth returns an OAuth service from the context
func GetCtxOAuth(inj governor.Injector) OAuth {
	v := inj.Get(ctxKeyOAuth{})
	if v == nil {
		return nil
	}
	return v.(OAuth)
}

// setCtxOAuth sets an OAuth service in the context
func setCtxOAuth(inj governor.Injector, o OAuth) {
	inj.Set(ctxKeyOAuth{}, o)
}

// NewCtx creates a new OAuth service from a context
func NewCtx(inj governor.Injector) *Service {
	return New(
		oauthappmodel.GetCtxRepo(inj),
		oauthconnmodel.GetCtxRepo(inj),
		token.GetCtxTokenizer(inj),
		kvstore.GetCtxKVStore(inj),
		objstore.GetCtxBucket(inj),
		user.GetCtxUsers(inj),
		events.GetCtxEvents(inj),
		ratelimit.GetCtxRatelimiter(inj),
		gate.GetCtxGate(inj),
	)
}

// New returns a new Apikey
func New(
	apps oauthappmodel.Repo,
	connections oauthconnmodel.Repo,
	tokenizer token.Tokenizer,
	kv kvstore.KVStore,
	obj objstore.Bucket,
	users user.Users,
	ev events.Events,
	ratelimiter ratelimit.Ratelimiter,
	g gate.Gate,
) *Service {
	return &Service{
		apps:        apps,
		connections: connections,
		tokenizer:   tokenizer,
		kvclient:    kv.Subtree("client"),
		oauthBucket: obj,
		logoImgDir:  obj.Subdir("logo"),
		users:       users,
		events:      ev,
		ratelimiter: ratelimiter,
		gate:        g,
		wg:          ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxOAuth(inj, s)
	s.rolens = "gov." + r.Name()
	s.scopens = "gov." + r.Name()
	s.streamns = r.Name()

	r.SetDefault("codeduration", "1m")
	r.SetDefault("accessduration", "5m")
	r.SetDefault("refreshduration", "168h")
	r.SetDefault("keycache", "24h")
	r.SetDefault("realm", "governor")
	r.SetDefault("ephost", "http://localhost:8080")
	r.SetDefault("epprofile", "http://localhost:8080/u/{{.Username}}")
	r.SetDefault("eppicture", "http://localhost:8080/api/profile/id/{{.Userid}}/image")
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	var err error
	s.codeDuration, err = r.GetDuration("codeduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse code duration")
	}
	s.accessDuration, err = r.GetDuration("accessduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse access duration")
	}
	s.refreshDuration, err = r.GetDuration("refreshduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse refresh duration")
	}
	s.keyCache, err = r.GetDuration("keycache")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse key cache duration")
	}

	s.realm = r.GetStr("realm")
	s.issuer = r.GetStr("issuer")
	s.epauth = r.GetStr("epauthorize")
	base := r.GetStr("ephost") + r.Config().BasePath + r.URL()
	s.eptoken = base + tokenRoute
	s.epuserinfo = base + userinfoRoute
	s.epjwks = base + jwksRoute

	if t, err := htmlTemplate.New("epprofile").Parse(r.GetStr("epprofile")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse profile url template")
	} else {
		s.tplprofile = t
	}
	if t, err := htmlTemplate.New("eppicture").Parse(r.GetStr("eppicture")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse profile picture url template")
	} else {
		s.tplpicture = t
	}

	s.log.Info(ctx, "Loaded config",
		klog.AString("codeduration", s.codeDuration.String()),
		klog.AString("accessduration", s.accessDuration.String()),
		klog.AString("refreshduration", s.refreshDuration.String()),
		klog.AString("keycache", s.keyCache.String()),
		klog.AString("issuer", s.issuer),
		klog.AString("authorization_endpoint", s.epauth),
		klog.AString("token_endpoint", s.eptoken),
		klog.AString("userinfo_endpoint", s.epuserinfo),
		klog.AString("jwks_uri", s.epjwks),
		klog.AString("profile_endpoint", r.GetStr("epprofile")),
		klog.AString("picture_endpoint", r.GetStr("eppicture")),
	)

	sr := s.router()
	sr.mountOidRoutes(m)
	sr.mountAppRoutes(m.Group("/app"))
	s.log.Info(ctx, "Mounted http routes")

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream")
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.apps.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created oauthapps table")

	if err := s.connections.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created oauthconnections table")

	if err := s.oauthBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init oauth bucket")
	}
	s.log.Info(ctx, "Created oauth bucket")

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func (s *Service) userEventHandler(ctx context.Context, props user.UserEvent) error {
	switch props.Kind {
	case user.UserEventKindDelete:
		return s.userDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userDeleteEventHandler(ctx context.Context, props user.DeleteUserProps) error {
	if err := s.deleteUserConnections(ctx, props.Userid); err != nil {
		return err
	}
	return nil
}
