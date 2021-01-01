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
	"xorkevin.dev/governor/service/user/oauth/connection/model"
	"xorkevin.dev/governor/service/user/oauth/model"
	"xorkevin.dev/governor/service/user/token"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// OAuth manages OAuth apps
	OAuth interface {
	}

	Service interface {
		governor.Service
		OAuth
	}

	service struct {
		apps         oauthmodel.Repo
		connections  connectionmodel.Repo
		tokenizer    token.Tokenizer
		kvclient     kvstore.KVStore
		logoBucket   objstore.Bucket
		logoImgDir   objstore.Dir
		gate         gate.Gate
		logger       governor.Logger
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
	apps := oauthmodel.GetCtxRepo(inj)
	connections := connectionmodel.GetCtxRepo(inj)
	tokenizer := token.GetCtxTokenizer(inj)
	kv := kvstore.GetCtxKVStore(inj)
	obj := objstore.GetCtxBucket(inj)
	g := gate.GetCtxGate(inj)
	return New(apps, connections, tokenizer, kv, obj, g)
}

// New returns a new Apikey
func New(apps oauthmodel.Repo, connections connectionmodel.Repo, tokenizer token.Tokenizer, kv kvstore.KVStore, obj objstore.Bucket, g gate.Gate) Service {
	return &service{
		apps:         apps,
		connections:  connections,
		tokenizer:    tokenizer,
		kvclient:     kv.Subtree("client"),
		logoBucket:   obj,
		logoImgDir:   obj.Subdir("logo"),
		gate:         g,
		keyCacheTime: time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxOAuth(inj, s)

	r.SetDefault("keycache", "24h")
	r.SetDefault("epbase", "http://localhost:8080/api/oauth")
	r.SetDefault("epauthorization", "/auth/authorize")
	r.SetDefault("eptoken", "/auth/token")
	r.SetDefault("epuserinfo", "/userinfo")
	r.SetDefault("epjwks", "/jwks")
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

	if t, err := time.ParseDuration(r.GetStr("keycache")); err != nil {
		return governor.NewError("Failed to parse key cache time", http.StatusBadRequest, err)
	} else {
		s.keyCacheTime = int64(t / time.Second)
	}

	s.issuer = r.GetStr("issuer")
	base := r.GetStr("epbase")
	s.epauth = base + r.GetStr("epauthorization")
	s.eptoken = base + r.GetStr("eptoken")
	s.epuserinfo = base + r.GetStr("epuserinfo")
	s.epjwks = base + r.GetStr("epjwks")

	l.Info("loaded config", map[string]string{
		"keycache (s)":           strconv.FormatInt(s.keyCacheTime, 10),
		"authorization_endpoint": s.epauth,
		"token_endpoint":         s.eptoken,
		"userinfo_endpoint":      s.epuserinfo,
		"jwks_uri":               s.epjwks,
	})

	sr := s.router()
	sr.mountRoutes(m)
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
