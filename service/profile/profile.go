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
)

type (
	// Profiles is a user profile management service
	Profiles interface {
	}

	// Service is a Profiles and governor.Service
	Service interface {
		governor.Service
		Profiles
	}

	service struct {
		profiles      model.Repo
		profileBucket objstore.Bucket
		profileDir    objstore.Dir
		events        events.Events
		ratelimiter   ratelimit.Ratelimiter
		gate          gate.Gate
		logger        governor.Logger
		scopens       string
		streamns      string
		useropts      user.Opts
	}

	router struct {
		s  *service
		rt governor.Middleware
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
func NewCtx(inj governor.Injector) Service {
	profiles := model.GetCtxRepo(inj)
	obj := objstore.GetCtxBucket(inj)
	ev := events.GetCtxEvents(inj)
	ratelimiter := ratelimit.GetCtxRatelimiter(inj)
	g := gate.GetCtxGate(inj)
	useropts := user.GetCtxOpts(inj)
	return New(profiles, obj, ev, ratelimiter, g, useropts)
}

// New creates a new Profiles service
func New(profiles model.Repo, obj objstore.Bucket, ev events.Events, ratelimiter ratelimit.Ratelimiter, g gate.Gate, useropts user.Opts) Service {
	return &service{
		profiles:      profiles,
		profileBucket: obj,
		profileDir:    obj.Subdir("profileimage"),
		events:        ev,
		ratelimiter:   ratelimiter,
		gate:          g,
		useropts:      useropts,
	}
}

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxProfiles(inj, s)
	s.scopens = "gov." + name
	s.streamns = strings.ToUpper(name)
}

func (s *service) router() *router {
	return &router{
		s:  s,
		rt: s.ratelimiter.Base(),
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	sr := s.router()
	sr.mountProfileRoutes(m)
	l.Info("Mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.profiles.Setup(context.Background()); err != nil {
		return err
	}
	l.Info("Created profile table", nil)
	if err := s.profileBucket.Init(context.Background()); err != nil {
		return kerrors.WithMsg(err, "Failed to init profile image bucket")
	}
	l.Info("Created profile bucket", nil)
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if _, err := s.events.StreamSubscribe(s.useropts.StreamName, s.useropts.CreateChannel, s.streamns+"_WORKER_CREATE", s.UserCreateHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user create queue")
	}
	l.Info("Subscribed to user create queue", nil)

	if _, err := s.events.StreamSubscribe(s.useropts.StreamName, s.useropts.DeleteChannel, s.streamns+"_WORKER_DELETE", s.UserDeleteHook, events.StreamConsumerOpts{
		AckWait:     15 * time.Second,
		MaxDeliver:  30,
		MaxPending:  1024,
		MaxRequests: 32,
	}); err != nil {
		return kerrors.WithMsg(err, "Failed to subscribe to user delete queue")
	}
	l.Info("Subscribed to user delete queue", nil)

	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// UserCreateHook creates a new profile for a new user
func (s *service) UserCreateHook(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := user.DecodeNewUserProps(msgdata)
	if err != nil {
		return err
	}
	if _, err := s.CreateProfile(ctx, props.Userid, "", ""); err != nil {
		return err
	}
	return nil
}

// UserDeleteHook deletes the profile of a deleted user
func (s *service) UserDeleteHook(ctx context.Context, pinger events.Pinger, topic string, msgdata []byte) error {
	props, err := user.DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	if err := s.DeleteProfile(ctx, props.Userid); err != nil {
		return err
	}
	return nil
}
