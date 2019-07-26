package profile

import (
	"github.com/labstack/echo"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/gate"
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
func New(conf governor.Config, l governor.Logger, repo profilemodel.Repo, obj objstore.Objstore, g gate.Gate, img image.Image, cc cachecontrol.CacheControl) (Profile, error) {
	b, err := obj.GetBucketDefLoc(imageBucket)
	if err != nil {
		l.Error("fail get profile picture bucket", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	l.Info("initialize profile service", nil)

	return &profileService{
		repo: repo,
		obj:  b,
		gate: g,
		img:  img,
		cc:   cc,
	}, nil
}

func (p *profileService) newRouter() *profileRouter {
	return &profileRouter{
		service: *p,
	}
}

// Mount is a collection of routes for accessing and modifying profile data
func (p *profileService) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	pr := p.newRouter()

	if err := pr.mountProfileRoutes(conf, r); err != nil {
		return err
	}

	l.Info("mount profile service", nil)
	return nil
}

// Health is a check for service health
func (p *profileService) Health() error {
	return nil
}

// Setup is run on service setup
func (p *profileService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	if err := p.repo.Setup(); err != nil {
		return err
	}
	l.Info("create profile table", nil)
	return nil
}

// UserCreateHook creates a new profile on new user
func (p *profileService) UserCreateHook(props user.HookProps) error {
	_, err := p.CreateProfile(props.Userid, "", "")
	return err
}

// UserDeleteHook deletes the profile on delete user
func (p *profileService) UserDeleteHook(userid string) error {
	return p.DeleteProfile(userid)
}
