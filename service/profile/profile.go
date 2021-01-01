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
	// Profile is a user profile management service
	Profile interface {
	}

	Service interface {
		governor.Service
		Profile
	}

	service struct {
		profiles      profilemodel.Repo
		profileBucket objstore.Bucket
		profileDir    objstore.Dir
		queue         msgqueue.Msgqueue
		gate          gate.Gate
		logger        governor.Logger
	}

	router struct {
		s service
	}

	ctxKeyProfile struct{}
)

// GetCtxProfile returns a Profile service from the context
func GetCtxProfile(ctx context.Context) (Profile, error) {
	v := ctx.Value(ctxKeyProfile{})
	if v == nil {
		return nil, governor.NewError("Profile serivce not found in context", http.StatusInternalServerError, nil)
	}
	return v.(Profile), nil
}

// SetCtxProfile sets a profile service in the context
func SetCtxProfile(ctx context.Context, p Profile) context.Context {
	return context.WithValue(ctx, ctxKeyProfile{}, p)
}

// NewCtx creates a new Profile service from a context
func NewCtx(ctx context.Context) (Service, error) {
	profiles, err := profilemodel.GetCtxRepo(ctx)
	if err != nil {
		return nil, err
	}
	obj, err := objstore.GetCtxBucket(ctx)
	if err != nil {
		return nil, err
	}
	queue, err := msgqueue.GetCtxMsgqueue(ctx)
	if err != nil {
		return nil, err
	}
	g, err := gate.GetCtxGate(ctx)
	if err != nil {
		return nil, err
	}
	return New(profiles, obj, queue, g), nil
}

// New creates a new Profile service
func New(profiles profilemodel.Repo, obj objstore.Bucket, queue msgqueue.Msgqueue, g gate.Gate) Service {
	return &service{
		profiles:      profiles,
		profileBucket: obj,
		profileDir:    obj.Subdir("profileimage"),
		queue:         queue,
		gate:          g,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
