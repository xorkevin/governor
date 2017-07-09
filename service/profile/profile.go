package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/profile/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	reqProfileGetID struct {
		Userid string `json:"userid"`
	}

	reqProfileModel struct {
		Userid string `json:"userid"`
		Email  string `json:"contact_email"`
		Bio    string `json:"bio"`
		Image  string `json:"image"`
	}

	resProfileModel struct {
		Userid []byte `json:"userid"`
		Email  string `json:"contact_email"`
		Bio    string `json:"bio"`
		Image  string `json:"image"`
	}
)

func (r *reqProfileGetID) valid() *governor.Error {
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	return nil
}

func (r *reqProfileModel) valid() *governor.Error {
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	if err := validEmail(r.Email); err != nil {
		return err
	}
	if err := validBio(r.Email); err != nil {
		return err
	}
	if err := validImage(r.Image); err != nil {
		return err
	}
	return nil
}

const (
	moduleID = "profile"
)

type (
	// Profile is a service for storing user profile information
	Profile struct {
		db    *db.Database
		cache *cache.Cache
	}
)

// New creates a new Profile service
func New(conf governor.Config, l *logrus.Logger, db *db.Database, ch *cache.Cache) *Profile {
	return &Profile{
		db:    db,
		cache: ch,
	}
}

// Mount is a collection of routes for accessing and modifying profile data
func (u *Profile) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := u.db.DB()

	r.POST("/", func(c echo.Context) error {
		rprofile := &reqProfileModel{
			Userid: c.Param("id"),
		}
		if err := rprofile.valid(); err != nil {
			return err
		}

		m := &profilemodel.Model{
			Email: rprofile.Email,
			Bio:   rprofile.Bio,
			Image: rprofile.Image,
		}

		if err := m.SetIDB64(rprofile.Userid); err != nil {
			err.SetErrorUser()
			return err
		}

		if err := m.Insert(db); err != nil {
			if err.Code() == 3 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	})

	r.PUT("/:id", func(c echo.Context) error {
		rprofile := &reqProfileGetID{
			Userid: c.Param("id"),
		}
		if err := rprofile.valid(); err != nil {
			return err
		}

		return c.NoContent(http.StatusNoContent)
	})

	l.Info("mounted profile service")

	return nil
}

// Health is a check for service health
func (u *Profile) Health() *governor.Error {
	return nil
}
