package oauth

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
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
	g := gate.GetCtxGate(inj)
	return New(apps, connections, tokenizer, kv, obj, g)
}

// New returns a new Apikey
func New(apps model.Repo, connections connmodel.Repo, tokenizer token.Tokenizer, kv kvstore.KVStore, obj objstore.Bucket, g gate.Gate) Service {
	return &service{
		apps:         apps,
		connections:  connections,
		tokenizer:    tokenizer,
		kvclient:     kv.Subtree("client"),
		logoBucket:   obj,
		logoImgDir:   obj.Subdir("logo"),
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
		return governor.NewError("Failed to parse code time", http.StatusBadRequest, err)
	} else {
		s.codeTime = int64(t / time.Second)
	}

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

	if t, err := time.ParseDuration(r.GetStr("keycache")); err != nil {
		return governor.NewError("Failed to parse key cache time", http.StatusBadRequest, err)
	} else {
		s.keyCacheTime = int64(t / time.Second)
	}

	s.issuer = r.GetStr("issuer")
	s.epauth = r.GetStr("epauthorize")
	base := r.GetStr("ephost") + c.BaseURL + r.URL()
	s.eptoken = base + tokenRoute
	s.epuserinfo = base + userinfoRoute
	s.epjwks = base + jwksRoute

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
		return governor.NewError("Failed to init oauth app logo bucket", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
