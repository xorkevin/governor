package profile

import (
	"context"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	profilequeueworkercreate = "gov.profile.worker.create"
	profilequeueworkerdelete = "gov.profile.worker.delete"
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
		queue         msgqueue.Msgqueue
		gate          gate.Gate
		logger        governor.Logger
	}

	router struct {
		s service
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
	queue := msgqueue.GetCtxMsgqueue(inj)
	g := gate.GetCtxGate(inj)
	return New(profiles, obj, queue, g)
}

// New creates a new Profiles service
func New(profiles model.Repo, obj objstore.Bucket, queue msgqueue.Msgqueue, g gate.Gate) Service {
	return &service{
		profiles:      profiles,
		profileBucket: obj,
		profileDir:    obj.Subdir("profileimage"),
		queue:         queue,
		gate:          g,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxProfiles(inj, s)
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

	sr := s.router()
	sr.mountProfileRoutes(m)
	l.Info("mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if err := s.profiles.Setup(); err != nil {
		return err
	}
	l.Info("created profile table", nil)
	return nil
}

func (s *service) Start(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})

	if err := s.profileBucket.Init(); err != nil {
		return governor.NewError("Failed to init profile image bucket", http.StatusInternalServerError, err)
	}

	if _, err := s.queue.Subscribe(user.NewUserQueueID, profilequeueworkercreate, 15*time.Second, 2, s.UserCreateHook); err != nil {
		return governor.NewError("Failed to subscribe to user create queue", http.StatusInternalServerError, err)
	}
	if _, err := s.queue.Subscribe(user.DeleteUserQueueID, profilequeueworkerdelete, 15*time.Second, 2, s.UserDeleteHook); err != nil {
		return governor.NewError("Failed to subscribe to user delete queue", http.StatusInternalServerError, err)
	}
	l.Info("subscribed to user create/delete queue", nil)
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// UserCreateHook creates a new profile for a new user
func (s *service) UserCreateHook(msgdata []byte) error {
	props, err := user.DecodeNewUserProps(msgdata)
	if err != nil {
		return err
	}
	if _, err := s.CreateProfile(props.Userid, "", ""); err != nil {
		return err
	}
	return nil
}

// UserDeleteHook deletes the profile of a deleted user
func (s *service) UserDeleteHook(msgdata []byte) error {
	props, err := user.DecodeDeleteUserProps(msgdata)
	if err != nil {
		return err
	}
	if err := s.DeleteProfile(props.Userid); err != nil {
		return err
	}
	return nil
}
