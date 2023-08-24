package profile

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/profilemodel"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Profiles is a user profile management service
	Profiles interface{}

	Service struct {
		profiles      profilemodel.Repo
		profileBucket objstore.Bucket
		profileDir    objstore.Dir
		users         user.Users
		ratelimiter   ratelimit.Ratelimiter
		gate          gate.Gate
		log           *klog.LevelLogger
		scopens       string
		streamns      string
		wg            *ksync.WaitGroup
	}

	router struct {
		s  *Service
		rt governor.MiddlewareCtx
	}
)

// New creates a new Profiles service
func New(profiles profilemodel.Repo, obj objstore.Bucket, users user.Users, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
	return &Service{
		profiles:      profiles,
		profileBucket: obj,
		profileDir:    obj.Subdir("profileimage"),
		users:         users,
		ratelimiter:   ratelimiter,
		gate:          g,
		wg:            ksync.NewWaitGroup(),
	}
}

func (s *Service) Register(r governor.ConfigRegistrar) {
	s.scopens = "gov." + r.Name()
	s.streamns = r.Name()
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	s.log = klog.NewLevelLogger(kit.Logger)

	sr := s.router()
	sr.mountProfileRoutes(kit.Router)
	s.log.Info(ctx, "Mounted http routes")
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream")
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"))
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.profiles.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created profile table")
	if err := s.profileBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init profile image bucket")
	}
	s.log.Info(ctx, "Created profile bucket")
	return nil
}

func (s *Service) Health(ctx context.Context) error {
	return nil
}

func (s *Service) userEventHandler(ctx context.Context, props user.UserEvent) error {
	switch props.Kind {
	case user.UserEventKindCreate:
		return s.userCreateEventHandler(ctx, props.Create)
	case user.UserEventKindDelete:
		return s.userDeleteEventHandler(ctx, props.Delete)
	default:
		return nil
	}
}

func (s *Service) userCreateEventHandler(ctx context.Context, props user.CreateUserProps) error {
	if _, err := s.createProfile(ctx, props.Userid, "", ""); err != nil {
		return err
	}
	return nil
}

func (s *Service) userDeleteEventHandler(ctx context.Context, props user.DeleteUserProps) error {
	if err := s.deleteProfile(ctx, props.Userid); err != nil {
		return err
	}
	return nil
}
