package courier

import (
	"context"
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
		repo          model.Repo
		kvlinks       kvstore.KVStore
		linkImgBucket objstore.Bucket
		linkImgDir    objstore.Dir
		brandImgDir   objstore.Dir
		gate          gate.Gate
		logger        governor.Logger
		fallbackLink  string
		linkPrefix    string
		cacheTime     int64
	}

	router struct {
		s service
	}

	ctxKeyCourier struct{}
)

// GetCtxCourier returns a Courier service from the context
func GetCtxCourier(inj governor.Injector) Courier {
	v := inj.Get(ctxKeyCourier{})
	if v == nil {
		return nil
	}
	return v.(Courier)
}

// setCtxCourier sets a Courier service in the context
func setCtxCourier(inj governor.Injector, c Courier) {
	inj.Set(ctxKeyCourier{}, c)
}

// NewCtx creates a new Courier service from a context
func NewCtx(inj governor.Injector) Service {
	repo := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	obj := objstore.GetCtxBucket(inj)
	g := gate.GetCtxGate(inj)
	return New(repo, kv, obj, g)
}

// New creates a new Courier service
func New(repo model.Repo, kv kvstore.KVStore, obj objstore.Bucket, g gate.Gate) Service {
	return &service{
		repo:          repo,
		kvlinks:       kv.Subtree("links"),
		linkImgBucket: obj,
		linkImgDir:    obj.Subdir("qr"),
		brandImgDir:   obj.Subdir("brand"),
		gate:          g,
		cacheTime:     time24h,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxCourier(inj, s)

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
		return governor.ErrWithMsg(err, "Failed to parse cache time")
	} else {
		s.cacheTime = int64(t / time.Second)
	}
	if len(s.fallbackLink) == 0 {
		l.Warn("fallbacklink is not set", nil)
	} else if err := validURL(s.fallbackLink); err != nil {
		return governor.ErrWithMsg(err, "Invalid fallbacklink")
	}
	if len(s.linkPrefix) == 0 {
		l.Warn("linkprefix is not set", nil)
	} else if err := validURL(s.linkPrefix); err != nil {
		return governor.ErrWithMsg(err, "Invalid linkprefix")
	}

	l.Info("loaded config", map[string]string{
		"fallbacklink": s.fallbackLink,
		"linkprefix":   s.linkPrefix,
		"cachetime":    strconv.FormatInt(s.cacheTime, 10),
	})

	sr := s.router()
	sr.mountRoutes(m)
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
		return governor.ErrWithMsg(err, "Failed to init courier link image bucket")
	}
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}
