package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/profile/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
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

	reqSetPublicFields struct {
		Add    []string `json:"add"`
		Remove []string `json:"remove"`
	}

	resProfileModel struct {
		Userid       []byte `json:"userid,omitempty"`
		Email        string `json:"contact_email"`
		Bio          string `json:"bio"`
		Image        string `json:"image"`
		PublicFields string `json:"public_fields"`
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

func (r *reqSetPublicFields) valid() *governor.Error {
	if err := validSetPublic(strings.Join(r.Add, ",")); err != nil {
		return err
	}
	if err := validSetPublic(strings.Join(r.Remove, ",")); err != nil {
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
		gate  *gate.Gate
	}
)

// New creates a new Profile service
func New(conf governor.Config, l *logrus.Logger, db *db.Database, ch *cache.Cache) *Profile {
	ca := conf.Conf().GetStringMapString("userauth")

	return &Profile{
		db:    db,
		cache: ch,
		gate:  gate.New(ca["secret"], ca["issuer"]),
	}
}

// Mount is a collection of routes for accessing and modifying profile data
func (p *Profile) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

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

		m.SetPublic([]string{"bio", "image"}, []string{})

		if err := m.Insert(db); err != nil {
			if err.Code() == 3 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.Owner("id"))

	r.PUT("/id/:id", func(c echo.Context) error {
		rprofile := &reqProfileModel{}
		if err := c.Bind(rprofile); err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		rprofile.Userid = c.Param("id")
		if err := rprofile.valid(); err != nil {
			return err
		}

		m, err := profilemodel.GetByIDB64(db, rprofile.Userid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
		}

		m.Email = rprofile.Email
		m.Bio = rprofile.Bio
		m.Image = rprofile.Image

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.Owner("id"))

	r.GET("/id/:id/private", func(c echo.Context) error {
		rprofile := &reqProfileGetID{
			Userid: c.Param("id"),
		}
		if err := rprofile.valid(); err != nil {
			return err
		}

		m, err := profilemodel.GetByIDB64(db, rprofile.Userid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
		}

		return c.JSON(http.StatusOK, &resProfileModel{
			Userid:       m.Userid,
			Email:        m.Email,
			Bio:          m.Bio,
			Image:        m.Image,
			PublicFields: m.PublicFields,
		})
	}, p.gate.OwnerOrAdmin("id"))

	r.GET("/id/:id", func(c echo.Context) error {
		rprofile := &reqProfileGetID{
			Userid: c.Param("id"),
		}
		if err := rprofile.valid(); err != nil {
			return err
		}

		m, err := profilemodel.GetByIDB64(db, rprofile.Userid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
		}

		return c.JSON(http.StatusOK, &resProfileModel{
			Email: m.Email,
			Bio:   m.Bio,
			Image: m.Image,
		})
	})

	l.Info("mounted profile service")

	return nil
}

// Health is a check for service health
func (p *Profile) Health() *governor.Error {
	return nil
}