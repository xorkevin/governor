package oauth

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/user/oauth/model"
)

type (
	// OAuthApp manages OAuth apps
	OAuthApp interface {
	}

	Service interface {
		governor.Service
		OAuthApp
	}

	service struct {
		oauthapps    oauthmodel.Repo
		kvkey        kvstore.KVStore
		logger       governor.Logger
		keyCacheTime int64
	}
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

// New returns a new Apikey
func New(oauthapps oauthmodel.Repo, kv kvstore.KVStore) Service {
	return &service{
		oauthapps:    oauthapps,
		kvkey:        kv.Subtree("key"),
		keyCacheTime: time24h,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("keycache", "24h")
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

	l.Info("loaded config", map[string]string{
		"keycache (s)": strconv.FormatInt(s.keyCacheTime, 10),
	})

	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})

	if err := s.oauthapps.Setup(); err != nil {
		return err
	}
	l.Info("created oauthapps table", nil)

	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}