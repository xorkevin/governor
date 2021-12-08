package courier

import (
	"context"
	"errors"
	"strconv"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	govworkerdelete = "DEV_XORKEVIN_GOV_COURIER_WORKER_DELETE"
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
		courierBucket objstore.Bucket
		linkImgDir    objstore.Dir
		brandImgDir   objstore.Dir
		events        events.Events
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
	ev := events.GetCtxEvents(inj)
	g := gate.GetCtxGate(inj)
	return New(repo, kv, obj, ev, g)
}

// New creates a new Courier service
func New(repo model.Repo, kv kvstore.KVStore, obj objstore.Bucket, ev events.Events, g gate.Gate) Service {
	return &service{
		repo:          repo,
		kvlinks:       kv.Subtree("links"),
		courierBucket: obj,
		linkImgDir:    obj.Subdir("qr"),
		brandImgDir:   obj.Subdir("brand"),
		events:        ev,
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
	l.Info("Mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.repo.Setup(); err != nil {
		return err
	}
	l.Info("Created courierlinks table", nil)

	if err := s.courierBucket.Init(); err != nil {
		return governor.ErrWithMsg(err, "Failed to init courier bucket")
	}
	l.Info("Created courier bucket", nil)
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(user.EventStream, user.DeleteChannel, govworkerdelete, s.UserDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return governor.ErrWithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

const (
	linkDeleteBatchSize = 256
)

// UserDeleteHook deletes the courier links and brands of a deleted user
func (s *service) UserDeleteHook(pinger events.Pinger, msgdata []byte) error {
	props, err := user.DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		links, err := s.GetLinkGroup(props.Userid, 256, 0)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get user links")
		}
		if len(links.Links) == 0 {
			break
		}
		linkids := make([]string, 0, len(links.Links))
		for _, i := range links.Links {
			if err := s.linkImgDir.Del(i.LinkID); err != nil {
				if !errors.Is(err, objstore.ErrNotFound{}) {
					return governor.ErrWithMsg(err, "Failed to delete qr code image")
				}
			}
			linkids = append(linkids, i.LinkID)
		}
		if err := s.repo.DeleteLinks(linkids); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete links")
		}
		if err := s.kvlinks.Del(linkids...); err != nil {
			s.logger.Error("Failed to delete linkid urls", map[string]string{
				"userid":     props.Userid,
				"error":      err.Error(),
				"actiontype": "linkcache",
			})
		}
		if len(linkids) < linkDeleteBatchSize {
			break
		}
	}
	for {
		if err := pinger.Ping(); err != nil {
			return err
		}
		brands, err := s.GetBrandGroup(props.Userid, 256, 0)
		if err != nil {
			return governor.ErrWithMsg(err, "Failed to get user brands")
		}
		if len(brands.Brands) == 0 {
			break
		}
		brandids := make([]string, 0, len(brands.Brands))
		for _, i := range brands.Brands {
			if err := s.brandImgDir.Del(i.BrandID); err != nil {
				if !errors.Is(err, objstore.ErrNotFound{}) {
					return governor.ErrWithMsg(err, "Failed to delete brand image")
				}
			}
			brandids = append(brandids, i.BrandID)
		}
		if err := s.repo.DeleteBrands(props.Userid, brandids); err != nil {
			return governor.ErrWithMsg(err, "Failed to delete brands")
		}
		if len(brandids) < linkDeleteBatchSize {
			break
		}
	}
	return nil
}
