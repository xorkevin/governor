package courier

import (
	"context"
	"fmt"
	"github.com/labstack/echo"
	"strconv"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	linkImageBucketID       = "courier-link-image"
	min1              int64 = 60
	b1                      = 1_000_000_000
	min15                   = 900
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
		repo            couriermodel.Repo
		objstore        objstore.Objstore
		linkImageBucket objstore.Bucket
		kv              kvstore.KVStore
		gate            gate.Gate
		logger          governor.Logger
		fallbackLink    string
		linkPrefix      string
		cacheTime       int64
	}

	router struct {
		s service
	}
)

// New creates a new Courier service
func New(repo couriermodel.Repo, obj objstore.Objstore, kv kvstore.KVStore, g gate.Gate) Service {
	return &service{
		repo:      repo,
		objstore:  obj,
		kv:        kv,
		gate:      g,
		cacheTime: min1,
	}
}

func (s *service) Register(r governor.ConfigRegistrar) {
	r.SetDefault("fallbacklink", "")
	r.SetDefault("linkprefix", "")
	r.SetDefault("cachetime", "1m")
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	conf := r.GetStrMap("")
	s.fallbackLink = conf["fallbacklink"]
	s.linkPrefix = conf["linkprefix"]
	if t, err := time.ParseDuration(conf["cachetime"]); err != nil {
		s.logger.Warn(fmt.Sprintf("courier: failed to parse cache time: %s", conf["cachetime"]), nil)
	} else {
		s.cacheTime = t.Nanoseconds() / b1
	}
	if len(s.fallbackLink) == 0 {
		s.logger.Warn("courier: fallbacklink is not set", nil)
	} else if err := validURL(s.fallbackLink); err != nil {
		s.logger.Error("invalid fallbacklink", map[string]string{
			"error": err.Error(),
		})
		return err
	}
	if len(s.linkPrefix) == 0 {
		s.logger.Warn("courier: linkprefix is not set", nil)
	} else if err := validURL(s.linkPrefix); err != nil {
		s.logger.Error("invalid linkprefix", map[string]string{
			"error": err.Error(),
		})
		return err
	}

	l.Info("courier: loaded config", map[string]string{
		"fallbacklink": s.fallbackLink,
		"linkprefix":   s.linkPrefix,
		"cachetime":    strconv.FormatInt(s.cacheTime, 10),
	})

	sr := s.router()
	if err := sr.mountRoutes(g); err != nil {
		return err
	}
	l.Info("courier: mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	if err := s.repo.Setup(); err != nil {
		return err
	}
	s.logger.Info("created courierlinks table", nil)
	return nil
}

func (s *service) Start(ctx context.Context) error {
	b, err := s.objstore.GetBucketDefLoc(linkImageBucketID)
	if err != nil {
		s.logger.Error("fail get courier link image bucket", map[string]string{
			"error": err.Error(),
		})
		return err
	}
	s.linkImageBucket = b
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
