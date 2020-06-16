package courier

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
	min15         = int64(15 * time.Minute / time.Second)
)

type (
	// Courier is a service for sharing information
	Courier interface {
	}

	// Service is the public interface for the courier service server
	Service interface {
		governor.Service
		Courier
	}

	service struct {
		repo          couriermodel.Repo
		linkImgBucket objstore.Bucket
		linkImgDir    objstore.Dir
		brandImgDir   objstore.Dir
		kvlinks       kvstore.KVStore
		gate          gate.Gate
		logger        governor.Logger
		fallbackLink  string
		linkPrefix    string
		cacheTime     int64
	}

	router struct {
		s service
	}
)

// New creates a new Courier service
func New(repo couriermodel.Repo, obj objstore.Bucket, kv kvstore.KVStore, g gate.Gate) Service {
	return &service{
		repo:          repo,
		linkImgBucket: obj,
		linkImgDir:    obj.Subdir("qr"),
		brandImgDir:   obj.Subdir("brand"),
		kvlinks:       kv.Subtree("links"),
		gate:          g,
		cacheTime:     time24h,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("fallbacklink", "")
	r.SetDefault("linkprefix", "")
	r.SetDefault("cachetime", "24h")
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

	s.fallbackLink = r.GetStr("fallbacklink")
	s.linkPrefix = r.GetStr("linkprefix")
	if t, err := time.ParseDuration(r.GetStr("cachetime")); err != nil {
		return governor.NewError("Failed to parse cache time", http.StatusBadRequest, nil)
	} else {
		s.cacheTime = int64(t / time.Second)
	}
	if len(s.fallbackLink) == 0 {
		l.Warn("fallbacklink is not set", nil)
	} else if err := validURL(s.fallbackLink); err != nil {
		return governor.NewError("Invalid fallbacklink", http.StatusBadRequest, err)
	}
	if len(s.linkPrefix) == 0 {
		l.Warn("linkprefix is not set", nil)
	} else if err := validURL(s.linkPrefix); err != nil {
		return governor.NewError("Invalid linkprefix", http.StatusBadRequest, err)
	}

	l.Info("loaded config", map[string]string{
		"fallbacklink": s.fallbackLink,
		"linkprefix":   s.linkPrefix,
		"cachetime":    strconv.FormatInt(s.cacheTime, 10),
	})

	sr := s.router()
	if err := sr.mountRoutes(m); err != nil {
		return err
	}
	l.Info("mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.repo.Setup(); err != nil {
		return err
	}
	l.Info("created courierlinks table", nil)
	return nil
}

func (s *service) Start(ctx context.Context) error {
	if err := s.linkImgBucket.Init(); err != nil {
		return governor.NewError("Failed to init courier link image bucket", http.StatusInternalServerError, err)
	}
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
