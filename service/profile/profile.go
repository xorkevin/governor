package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	reqUserGetID struct {
		Userid string `json:"userid"`
	}
)

func (r *reqUserGetID) valid() *governor.Error {
	return hasUserid(r.Userid)
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
	r.POST("/:id", func(c echo.Context) error {
		ruser := &reqUserGetID{
			Userid: c.Param("id"),
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		return c.NoContent(http.StatusNoContent)
	})

	r.PUT("/:id", func(c echo.Context) error {
		ruser := &reqUserGetID{
			Userid: c.Param("id"),
		}
		if err := ruser.valid(); err != nil {
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
