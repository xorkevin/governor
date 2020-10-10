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
		logoBucket   objstore.Bucket
		logoImgDir   objstore.Dir
		kvclient     kvstore.KVStore
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
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

// New returns a new Apikey
func New(apps oauthmodel.Repo, connections connectionmodel.Repo, tokenizer token.Tokenizer, obj objstore.Bucket, kv kvstore.KVStore, g gate.Gate) Service {
	return &service{
		apps:         apps,
		connections:  connections,
		tokenizer:    tokenizer,
		logoBucket:   obj,
		logoImgDir:   obj.Subdir("logo"),
		kvclient:     kv.Subtree("client"),
		gate:         g,
		keyCacheTime: time24h,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
