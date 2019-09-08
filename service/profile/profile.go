package profile

import (
	"context"
	"github.com/labstack/echo"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
)

const (
	imageBucket = "profile-image"
	min1        = 60
	min15       = 900
)

type (
	Service interface {
		governor.Service
		user.Hook
	}

	service struct {
		profiles profilemodel.Repo
		objstore objstore.Objstore
		obj      objstore.Bucket
		gate     gate.Gate
		logger   governor.Logger
	}

	router struct {
		s service
	}
)

// New creates a new Profile service
func New(profiles profilemodel.Repo, obj objstore.Objstore, g gate.Gate) Service {
	return &service{
		profiles: profiles,
		objstore: obj,
		gate:     g,
	}
}

func (s *service) Register(r governor.ConfigRegistrar) {
}

func (s *service) router() *router {
	return &router{
		s: *s,
	}
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l

	sr := s.router()
	if err := sr.mountProfileRoutes(g); err != nil {
		return err
	}
	s.logger.Info("profile: mounted http routes", nil)
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	if err := s.profiles.Setup(); err != nil {
		return err
	}
	s.logger.Info("profile: created profile table", nil)
	return nil
}

func (s *service) Start(ctx context.Context) error {
	b, err := s.objstore.GetBucketDefLoc(imageBucket)
	if err != nil {
		s.logger.Error("profile: failed get profile picture bucket", map[string]string{
			"error": err.Error(),
		})
		return err
	}
	s.obj = b
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// UserCreateHook implements user.Hook by creating a new profile for a new user
func (s *service) UserCreateHook(props user.HookProps) error {
	_, err := s.CreateProfile(props.Userid, "", "")
	return err
}

// UserDeleteHook implements user.Hook by deleting the profile of a deleted user
func (s *service) UserDeleteHook(userid string) error {
	return s.DeleteProfile(userid)
}
