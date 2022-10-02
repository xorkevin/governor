package profile

import (
	"context"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Profiles is a user profile management service
	Profiles interface {
	}

	Service struct {
		profiles      model.Repo
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

	ctxKeyProfiles struct{}
)

// GetCtxProfiles returns a Profiles service from the context
func GetCtxProfiles(inj governor.Injector) Profiles {
	v := inj.Get(ctxKeyProfiles{})
	if v == nil {
		return nil
	}
	return v.(Profiles)
}

// setCtxProfiles sets a profile service in the context
func setCtxProfiles(inj governor.Injector, p Profiles) {
	inj.Set(ctxKeyProfiles{}, p)
}

// NewCtx creates a new Profiles service from a context
func NewCtx(inj governor.Injector) *Service {
	return New(
		model.GetCtxRepo(inj),
		objstore.GetCtxBucket(inj),
		user.GetCtxUsers(inj),
		ratelimit.GetCtxRatelimiter(inj),
		gate.GetCtxGate(inj),
	)
}

// New creates a new Profiles service
func New(profiles model.Repo, obj objstore.Bucket, users user.Users, ratelimiter ratelimit.Ratelimiter, g gate.Gate) *Service {
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

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxProfiles(inj, s)
	s.scopens = "gov." + name
	s.streamns = name
}

func (s *Service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.BaseCtx(),
	}
}

func (s *Service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, log klog.Logger, m governor.Router) error {
	s.log = klog.NewLevelLogger(log)

	sr := s.router()
	sr.mountProfileRoutes(m)
	s.log.Info(ctx, "Mounted http routes", nil)
	return nil
}

func (s *Service) Start(ctx context.Context) error {
	s.wg.Add(1)
	go s.users.WatchUsers(s.streamns+".worker.users", events.ConsumerOpts{}, s.userEventHandler, nil, 0).Watch(ctx, s.wg, events.WatchOpts{})
	s.log.Info(ctx, "Subscribed to users stream", nil)
	return nil
}

func (s *Service) Stop(ctx context.Context) {
	if err := s.wg.Wait(ctx); err != nil {
		s.log.WarnErr(ctx, kerrors.WithMsg(err, "Failed to stop"), nil)
	}
}

func (s *Service) Setup(ctx context.Context, req governor.ReqSetup) error {
	if err := s.profiles.Setup(ctx); err != nil {
		return err
	}
	s.log.Info(ctx, "Created profile table", nil)
	if err := s.profileBucket.Init(ctx); err != nil {
		return kerrors.WithMsg(err, "Failed to init profile image bucket")
	}
	s.log.Info(ctx, "Created profile bucket", nil)
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
