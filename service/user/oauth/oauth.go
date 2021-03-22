package oauth

import (
	"context"
	htmlTemplate "html/template"
	"strconv"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	connmodel "xorkevin.dev/governor/service/user/oauth/connection/model"
	"xorkevin.dev/governor/service/user/oauth/model"
	"xorkevin.dev/governor/service/user/token"
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

	// Service is an OAuth and governor.Service
	Service interface {
		governor.Service
		OAuth
	}

	service struct {
		apps         model.Repo
		connections  connmodel.Repo
		tokenizer    token.Tokenizer
		kvclient     kvstore.KVStore
		logoBucket   objstore.Bucket
		logoImgDir   objstore.Dir
		users        user.Users
		gate         gate.Gate
		logger       governor.Logger
		codeTime     int64
		accessTime   int64
		refreshTime  int64
		keyCacheTime int64
		issuer       string
		epauth       string
		eptoken      string
		epuserinfo   string
		epjwks       string
		tplprofile   *htmlTemplate.Template
		tplpicture   *htmlTemplate.Template
	}

	router struct {
		s service
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
func NewCtx(inj governor.Injector) Service {
	apps := model.GetCtxRepo(inj)
	connections := connmodel.GetCtxRepo(inj)
	tokenizer := token.GetCtxTokenizer(inj)
	kv := kvstore.GetCtxKVStore(inj)
	obj := objstore.GetCtxBucket(inj)
	users := user.GetCtxUsers(inj)
	g := gate.GetCtxGate(inj)
	return New(apps, connections, tokenizer, kv, obj, users, g)
}

// New returns a new Apikey
func New(apps model.Repo, connections connmodel.Repo, tokenizer token.Tokenizer, kv kvstore.KVStore, obj objstore.Bucket, users user.Users, g gate.Gate) Service {
	return &service{
		apps:         apps,
		connections:  connections,
		tokenizer:    tokenizer,
		kvclient:     kv.Subtree("client"),
		logoBucket:   obj,
		logoImgDir:   obj.Subdir("logo"),
		users:        users,
		gate:         g,
		codeTime:     time1m,
		accessTime:   time5m,
		refreshTime:  time7d,
		keyCacheTime: time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxOAuth(inj, s)

	r.SetDefault("codetime", "1m")
	r.SetDefault("accesstime", "5m")
	r.SetDefault("refreshtime", "168h")
	r.SetDefault("keycache", "24h")
	r.SetDefault("ephost", "http://localhost:8080")
	r.SetDefault("epprofile", "http://localhost:8080/u/{{.Username}}")
	r.SetDefault("eppicture", "http://localhost:8080/api/profile/id/{{.Userid}}/image")
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

	if t, err := time.ParseDuration(r.GetStr("codetime")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse code time")
	} else {
		s.codeTime = int64(t / time.Second)
	}

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

	if t, err := time.ParseDuration(r.GetStr("keycache")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse key cache time")
	} else {
		s.keyCacheTime = int64(t / time.Second)
	}

	s.issuer = r.GetStr("issuer")
	s.epauth = r.GetStr("epauthorize")
	base := r.GetStr("ephost") + c.BaseURL + r.URL()
	s.eptoken = base + tokenRoute
	s.epuserinfo = base + userinfoRoute
	s.epjwks = base + jwksRoute

	if t, err := htmlTemplate.New("epprofile").Parse(r.GetStr("epprofile")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse profile url template")
	} else {
		s.tplprofile = t
	}
	if t, err := htmlTemplate.New("eppicture").Parse(r.GetStr("eppicture")); err != nil {
		return governor.ErrWithMsg(err, "Failed to parse profile picture url template")
	} else {
		s.tplpicture = t
	}

	l.Info("loaded config", map[string]string{
		"codetime (s)":           strconv.FormatInt(s.codeTime, 10),
		"accesstime (s)":         strconv.FormatInt(s.accessTime, 10),
		"refreshtime (s)":        strconv.FormatInt(s.refreshTime, 10),
		"keycache (s)":           strconv.FormatInt(s.keyCacheTime, 10),
		"issuer":                 s.issuer,
		"authorization_endpoint": s.epauth,
		"token_endpoint":         s.eptoken,
		"userinfo_endpoint":      s.epuserinfo,
		"jwks_uri":               s.epjwks,
		"profile_endpoint":       r.GetStr("epprofile"),
		"picture_endpoint":       r.GetStr("eppicture"),
	})

	sr := s.router()
	sr.mountOidRoutes(m)
	sr.mountAppRoutes(m.Group("/app"))
	l.Info("mounted http routes", nil)

	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.apps.Setup(); err != nil {
		return err
	}
	l.Info("created oauthapps table", nil)

	if err := s.connections.Setup(); err != nil {
		return err
	}
	l.Info("created oauthconnections table", nil)

	return nil
}

func (s *service) Start(ctx context.Context) error {
	if err := s.logoBucket.Init(); err != nil {
		return governor.ErrWithMsg(err, "Failed to init oauth app logo bucket")
	}
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
