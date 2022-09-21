package oauth

import (
	"context"
	htmlTemplate "html/template"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	connmodel "xorkevin.dev/governor/service/user/oauth/connection/model"
	"xorkevin.dev/governor/service/user/oauth/model"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	tokenRoute    = "/token"
	userinfoRoute = "/userinfo"
	jwksRoute     = "/jwks"
)

const (
	time1m  int64 = int64(time.Minute / time.Second)
	time5m  int64 = time1m * 5
	time24h int64 = int64(24 * time.Hour / time.Second)
	time7d  int64 = time24h * 7
)

type (
	// OAuth manages OAuth apps
	OAuth interface {
	}

	Service struct {
		apps         model.Repo
		connections  connmodel.Repo
		tokenizer    token.Tokenizer
		kvclient     kvstore.KVStore
		oauthBucket  objstore.Bucket
		logoImgDir   objstore.Dir
		users        user.Users
		events       events.Events
		ratelimiter  ratelimit.Ratelimiter
		gate         gate.Gate
		log          *klog.LevelLogger
		rolens       string
		scopens      string
		streamns     string
		codeTime     int64
		accessTime   int64
		refreshTime  int64
		keyCacheTime int64
		realm        string
		issuer       string
		epauth       string
		eptoken      string
		epuserinfo   string
		epjwks       string
		tplprofile   *htmlTemplate.Template
		tplpicture   *htmlTemplate.Template
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
	apps := model.GetCtxRepo(inj)
	connections := connmodel.GetCtxRepo(inj)
	tokenizer := token.GetCtxTokenizer(inj)
	kv := kvstore.GetCtxKVStore(inj)
	obj := objstore.GetCtxBucket(inj)
	users := user.GetCtxUsers(inj)
	ev := events.GetCtxEvents(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	return New(apps, connections, tokenizer, kv, obj, users, ev, ratelimiter, g)
}

// New returns a new Apikey
func New(
	apps model.Repo,
	connections connmodel.Repo,
	tokenizer token.Tokenizer,
	kv kvstore.KVStore,
	obj objstore.Bucket,
	users user.Users,
	ev events.Events,
	ratelimiter ratelimit.Ratelimiter,
	g gate.Gate,
) *Service {
	return &Service{
		apps:         apps,
		connections:  connections,
		tokenizer:    tokenizer,
		kvclient:     kv.Subtree("client"),
		oauthBucket:  obj,
		logoImgDir:   obj.Subdir("logo"),
		users:        users,
		events:       ev,
		ratelimiter:  ratelimiter,
		gate:         g,
		codeTime:     time1m,
		accessTime:   time5m,
		refreshTime:  time7d,
		keyCacheTime: time24h,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxOAuth(inj, s)
	s.rolens = "gov." + name
	s.scopens = "gov." + name
	s.streamns = strings.ToUpper(name)

	r.SetDefault("codetime", "1m")
	r.SetDefault("accesstime", "5m")
	r.SetDefault("refreshtime", "168h")
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

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	if t, err := time.ParseDuration(r.GetStr("codetime")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse code time")
	} else {
		s.codeTime = int64(t / time.Second)
	}

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

	if t, err := time.ParseDuration(r.GetStr("keycache")); err != nil {
		return kerrors.WithMsg(err, "Failed to parse key cache time")
	} else {
		s.keyCacheTime = int64(t / time.Second)
	}

	s.realm = r.GetStr("realm")
	s.issuer = r.GetStr("issuer")
	s.epauth = r.GetStr("epauthorize")
	base := r.GetStr("ephost") + c.BaseURL + r.URL()
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

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"oauth.codetime":               s.codeTime,
		"oauth.accesstime":             s.accessTime,
		"oauth.refreshtime":            s.refreshTime,
		"oauth.keycache":               s.keyCacheTime,
		"oauth.issuer":                 s.issuer,
		"oauth.authorization_endpoint": s.epauth,
		"oauth.token_endpoint":         s.eptoken,
		"oauth.userinfo_endpoint":      s.epuserinfo,
		"oauth.jwks_uri":               s.epjwks,
		"oauth.profile_endpoint":       r.GetStr("epprofile"),
		"oauth.picture_endpoint":       r.GetStr("eppicture"),
	})

	sr := s.router()
	sr.mountOidRoutes(m)
	sr.mountAppRoutes(m.Group("/app"))
	s.log.Info(ctx, "Mounted http routes", nil)

	return nil
}

func (s *Service) Start(ctx context.Context) error {
	if _, err := s.users.StreamSubscribeDelete(s.streamns+"_WORKER_DELETE", s.userDeleteHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	s.log.Info(ctx, "Subscribed to user delete queue", nil)
	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.apps.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created oauthapps table", nil)

	if err := s.connections.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created oauthconnections table", nil)

	if err := s.oauthBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init oauth bucket")
	}
	s.log.Info(ctx, "Created oauth bucket", nil)

	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func (s *Service) userDeleteHook(ctx context.Context, pinger events.Pinger, props user.DeleteUserProps) error {
	if err := s.deleteUserConnections(ctx, props.Userid); err != nil {
		return err
	}
	return nil
}
