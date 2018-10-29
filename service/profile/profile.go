package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/image"
	"github.com/hackform/governor/service/objstore"
	"github.com/hackform/governor/service/profile/model"
	"github.com/hackform/governor/service/user"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

const (
	moduleID    = "profile"
	imageBucket = "profile-image"
	min1        = 60
	min15       = 900
)

type (
	// Profile is a service for storing user profile information
	Profile interface {
		governor.Service
		user.Hook
	}

	profileService struct {
		repo profilemodel.Repo
		obj  objstore.Bucket
		gate gate.Gate
		img  image.Image
		cc   cachecontrol.CacheControl
	}

	profileRouter struct {
		service profileService
	}
)

// New creates a new Profile service
func New(conf governor.Config, l *logrus.Logger, repo profilemodel.Repo, obj objstore.Objstore, g gate.Gate, img image.Image, cc cachecontrol.CacheControl) Profile {
	b, err := obj.GetBucketDefLoc(imageBucket)
	if err != nil {
		l.Errorf("failed to get bucket: %s\n", err.Error())
	}

	l.Infof("profile: image bucket: %s", imageBucket)
	l.Info("initialized profile service")

	return &profileService{
		repo: repo,
		obj:  b,
		gate: g,
		img:  img,
		cc:   cc,
	}
}

func (p *profileService) newRouter() *profileRouter {
	return &profileRouter{
		service: *p,
	}
}

// Mount is a collection of routes for accessing and modifying profile data
func (p *profileService) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	pr := p.newRouter()

	if err := pr.mountProfileRoutes(conf, r); err != nil {
		return err
	}

	l.Info("mounted profile service")
	return nil
}

// Health is a check for service health
func (p *profileService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (p *profileService) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if err := p.repo.Setup(); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new profile table")
	return nil
}

// UserCreateHook creates a new profile on new user
func (p *profileService) UserCreateHook(props user.HookProps) *governor.Error {
	if _, err := p.CreateProfile(props.Userid, "", ""); err != nil {
		return err
	}

	return nil
}

// UserDeleteHook deletes the profile on delete user
func (p *profileService) UserDeleteHook(userid string) *governor.Error {
	return p.DeleteProfile(userid)
}
