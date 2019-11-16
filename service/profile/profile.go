package profile

import (
	"context"
	"github.com/labstack/echo/v4"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	profilequeueworkercreate = "profile-worker-create"
	profilequeueworkerdelete = "profile-worker-delete"
)

const (
	min1  = 60
	min15 = 900
)

type (
	Service interface {
		governor.Service
	}

	service struct {
		profiles      profilemodel.Repo
		objstore      objstore.Objstore
		profileBucket objstore.Bucket
		profileDir    objstore.Dir
		queue         msgqueue.Msgqueue
		gate          gate.Gate
		logger        governor.Logger
	}

	router struct {
		s service
	}
)

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

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	sr := s.router()
	if err := sr.mountProfileRoutes(g); err != nil {
		return err
	}
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

	if _, err := s.queue.SubscribeQueue(user.NewUserQueueID, profilequeueworkercreate, s.UserCreateHook); err != nil {
		return governor.NewError("Failed to subscribe to user create queue", http.StatusInternalServerError, err)
	}
	if _, err := s.queue.SubscribeQueue(user.DeleteUserQueueID, profilequeueworkerdelete, s.UserDeleteHook); err != nil {
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
func (s *service) UserCreateHook(msgdata []byte) {
	props, err := user.DecodeNewUserProps(msgdata)
	if err != nil {
		s.logger.Error("failed to decode new user props", map[string]string{
			"error":      err.Error(),
			"actiontype": "profiledecodenewuserprops",
		})
		return
	}
	if _, err := s.CreateProfile(props.Userid, "", ""); err != nil {
		s.logger.Error("failed to create new user from props", map[string]string{
			"error":      err.Error(),
			"actiontype": "profilecreateuserfromprops",
		})
	}
}

// UserDeleteHook deletes the profile of a deleted user
func (s *service) UserDeleteHook(msgdata []byte) {
	props, err := user.DecodeDeleteUserProps(msgdata)
	if err != nil {
		s.logger.Error("failed to decode delete user props", map[string]string{
			"error":      err.Error(),
			"actiontype": "profiledecodedeleteuserprops",
		})
		return
	}
	if err := s.DeleteProfile(props.Userid); err != nil {
		s.logger.Error("failed to delete user from props", map[string]string{
			"error":      err.Error(),
			"actiontype": "profiledeleteuserfromprops",
		})
	}
}
