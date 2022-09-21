package profile

import (
	"context"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
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
	profiles := model.GetCtxRepo(inj)
	obj := objstore.GetCtxBucket(inj)
	users := user.GetCtxUsers(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	return New(profiles, obj, users, ratelimiter, g)
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
	}
}

func (s *Service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar) {
	setCtxProfiles(inj, s)
	s.scopens = "gov." + name
	s.streamns = strings.ToUpper(name)
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
	if _, err := s.users.StreamSubscribeCreate(s.streamns+"_WORKER_CREATE", s.userCreateHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user create queue")
	}
	s.log.Info(ctx, "Subscribed to user create queue", nil)

	if _, err := s.users.StreamSubscribeDelete(s.streamns+"_WORKER_DELETE", s.userDeleteHook, events.StreamConsumerOpts{
		AckWait:    15 * time.Second,
		MaxDeliver: 30,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	s.log.Info(ctx, "Subscribed to user delete queue", nil)

	return nil
}

func (s *Service) Stop(ctx context.Context) {
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

func (s *Service) userCreateHook(ctx context.Context, pinger events.Pinger, props user.NewUserProps) error {
	if _, err := s.createProfile(ctx, props.Userid, "", ""); err != nil {
		return err
	}
	return nil
}

func (s *Service) userDeleteHook(ctx context.Context, pinger events.Pinger, props user.DeleteUserProps) error {
	if err := s.deleteProfile(ctx, props.Userid); err != nil {
		return err
	}
	return nil
}
