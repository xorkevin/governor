package courier

import (
	"context"
	"errors"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier/couriermodel"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/rank"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Courier is a service for sharing information
	Courier interface{}

	Service struct {
		repo          couriermodel.Repo
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
		wg            *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}
)

// New creates a new Courier service
func New(
	repo couriermodel.Repo,
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
		wg:            ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	s.scopens = "gov." + r.Name()
	s.streamns = r.Name()

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

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)

	s.fallbackLink = r.GetStr("fallbacklink")
	s.linkPrefix = r.GetStr("linkprefix")
	var err error
	s.cacheDuration, err = r.GetDuration("cacheduration")
	if err != nil {
		return kerrors.WithMsg(err, "Failed to parse cache time")
	}
	if s.fallbackLink == "" {
		s.log.Warn(ctx, "fallbacklink is not set")
	} else if err := validURL(s.fallbackLink); err != nil {
		return kerrors.WithMsg(err, "Invalid fallbacklink")
	}
	if s.linkPrefix == "" {
		s.log.Warn(ctx, "linkprefix is not set")
	} else if err := validURL(s.linkPrefix); err != nil {
		return kerrors.WithMsg(err, "Invalid linkprefix")
	}

	s.log.Info(ctx, "Loaded config",
		klog.AString("fallbacklink", s.fallbackLink),
		klog.AString("linkprefix", s.linkPrefix),
		klog.AString("cachetime", s.cacheDuration.String()),
	)

	sr := s.router()
	sr.mountRoutes(kit.Router)
	s.log.Info(ctx, "Mounted http routes")
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream")

	s.wg.Add(1)
	go s.orgs.WatchOrgs(s.streamns+".worker.orgs", events.ConsumerOpts{}, s.orgEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to orgs stream")

	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.repo.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created courierlinks table")

	if err := s.courierBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init courier bucket")
	}
	s.log.Info(ctx, "Created courier bucket")
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

const (
	linkDeleteBatchSize = 256
)

func (s *Service) userEventHandler(ctx context.Context, props user.UserEvent) error {
	switch props.Kind {
	case user.UserEventKindDelete:
		return s.userDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userDeleteEventHandler(ctx context.Context, props user.DeleteUserProps) error {
	return s.creatorDeleteEventHandler(ctx, props.Userid)
}

func (s *Service) orgEventHandler(ctx context.Context, props org.OrgEvent) error {
	switch props.Kind {
	case org.OrgEventKindDelete:
		return s.orgDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) orgDeleteEventHandler(ctx context.Context, props org.DeleteOrgProps) error {
	return s.creatorDeleteEventHandler(ctx, rank.ToOrgName(props.OrgID))
}

func (s *Service) creatorDeleteEventHandler(ctx context.Context, creatorid string) error {
	for {
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
				if !errors.Is(err, objstore.ErrNotFound) {
					return kerrors.WithMsg(err, "Failed to delete qr code image")
				}
			}
			linkids = append(linkids, i.LinkID)
		}
		if err := s.repo.DeleteLinks(ctx, linkids); err != nil {
			return kerrors.WithMsg(err, "Failed to delete links")
		}
		if err := s.kvlinks.Del(ctx, linkids...); err != nil {
			s.log.Error(ctx, "Failed to delete linkid urls",
				klog.AString("creatorid", creatorid),
			)
		}
		if len(linkids) < linkDeleteBatchSize {
			break
		}
	}
	for {
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
				if !errors.Is(err, objstore.ErrNotFound) {
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
