package courier

import (
	"context"
	"errors"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	time24h int64 = int64(24 * time.Hour / time.Second)
)

type (
	// Courier is a service for sharing information
	Courier interface {
	}

	Service struct {
		repo          model.Repo
		kvlinks       kvstore.KVStore
		courierBucket objstore.Bucket
		linkImgDir    objstore.Dir
		brandImgDir   objstore.Dir
		users         user.Users
		orgs          org.Orgs
		ratelimiter   ratelimit.Ratelimiter
		gate          gate.Gate
		log           *klog.LevelLogger
		scopens       string
		streamns      string
		fallbackLink  string
		linkPrefix    string
		cacheDuration time.Duration
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
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
func NewCtx(inj governor.Injector) *Service {
	repo := model.GetCtxRepo(inj)
	kv := kvstore.GetCtxKVStore(inj)
	obj := objstore.GetCtxBucket(inj)
	users := user.GetCtxUsers(inj)
	orgs := org.GetCtxOrgs(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	return New(repo, kv, obj, users, orgs, ratelimiter, g)
}

// New creates a new Courier service
func New(
	repo model.Repo,
	kv kvstore.KVStore,
	obj objstore.Bucket,
	users user.Users,
	orgs org.Orgs,
	ratelimiter ratelimit.Ratelimiter,
	g gate.Gate,
) *Service {
	return &Service{
		repo:          repo,
		kvlinks:       kv.Subtree("links"),
		courierBucket: obj,
		linkImgDir:    obj.Subdir("qr"),
		brandImgDir:   obj.Subdir("brand"),
		users:         users,
		orgs:          orgs,
		ratelimiter:   ratelimiter,
		gate:          g,
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxCourier(inj, s)
	s.scopens = "gov." + name
	s.streamns = strings.ToUpper(name)

	r.SetDefault("fallbacklink", "")
	r.SetDefault("linkprefix", "")
	r.SetDefault("cacheduration", "24h")
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	s.fallbackLink = r.GetStr("fallbacklink")
	s.linkPrefix = r.GetStr("linkprefix")
	var err error
	s.cacheDuration, err = r.GetDuration("cacheduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse cache time")
	}
	if s.fallbackLink == "" {
		s.log.Warn(ctx, "fallbacklink is not set", nil)
	} else if err := validURL(s.fallbackLink); err != nil {
		return kerrors.WithMsg(err, "Invalid fallbacklink")
	}
	if s.linkPrefix == "" {
		s.log.Warn(ctx, "linkprefix is not set", nil)
	} else if err := validURL(s.linkPrefix); err != nil {
		return kerrors.WithMsg(err, "Invalid linkprefix")
	}

	s.log.Info(ctx, "Loaded config", klog.Fields{
		"courier.fallbacklink": s.fallbackLink,
		"courier.linkprefix":   s.linkPrefix,
		"courier.cachetime":    s.cacheDuration.String(),
	})

	sr := s.router()
	sr.mountRoutes(m)
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

	if _, err := s.orgs.StreamSubscribeDelete(s.streamns+"_WORKER_ORG_DELETE", s.orgDeleteHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to org delete queue")
	}
	s.log.Info(ctx, "Subscribed to org delete queue", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.repo.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created courierlinks table", nil)

	if err := s.courierBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init courier bucket")
	}
	s.log.Info(ctx, "Created courier bucket", nil)
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

const (
	linkDeleteBatchSize = 256
)

func (s *Service) userDeleteHook(ctx context.Context, pinger events.Pinger, props user.DeleteUserProps) error {
	return s.creatorDeleteHook(ctx, pinger, props.Userid)
}

func (s *Service) orgDeleteHook(ctx context.Context, pinger events.Pinger, props org.DeleteOrgProps) error {
	return s.creatorDeleteHook(ctx, pinger, rank.ToOrgName(props.OrgID))
}

func (s *Service) creatorDeleteHook(ctx context.Context, pinger events.Pinger, creatorid string) error {
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		links, err := s.getLinkGroup(ctx, creatorid, linkDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get creator links")
		}
		if len(links.Links) == 0 {
			break
		}
		linkids := make([]string, 0, len(links.Links))
		for _, i := range links.Links {
			if err := s.linkImgDir.Del(ctx, i.LinkID); err != nil {
				if !errors.Is(err, objstore.ErrorNotFound{}) {
					return kerrors.WithMsg(err, "Failed to delete qr code image")
				}
			}
			linkids = append(linkids, i.LinkID)
		}
		if err := s.repo.DeleteLinks(ctx, linkids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete links")
		}
		if err := s.kvlinks.Del(ctx, linkids...); err != nil {
			s.log.Error(ctx, "Failed to delete linkid urls", klog.Fields{
				"courier.creatorid": creatorid,
			})
		}
		if len(linkids) < linkDeleteBatchSize {
			break
		}
	}
	for {
		if err := pinger.Ping(ctx); err != nil {
			return err
		}
		brands, err := s.getBrandGroup(ctx, creatorid, linkDeleteBatchSize, 0)
		if err != nil {
			return kerrors.WithMsg(err, "Failed to get creator brands")
		}
		if len(brands.Brands) == 0 {
			break
		}
		brandids := make([]string, 0, len(brands.Brands))
		for _, i := range brands.Brands {
			if err := s.brandImgDir.Del(ctx, i.BrandID); err != nil {
				if !errors.Is(err, objstore.ErrorNotFound{}) {
					return kerrors.WithMsg(err, "Failed to delete brand image")
				}
			}
			brandids = append(brandids, i.BrandID)
		}
		if err := s.repo.DeleteBrands(ctx, creatorid, brandids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete brands")
		}
		if len(brandids) < linkDeleteBatchSize {
			break
		}
	}
	return nil
}
