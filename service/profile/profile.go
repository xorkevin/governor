package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/objstore"
	"github.com/hackform/governor/service/profile/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"io"
	"mime"
	"net/http"
)

type (
	reqProfileGetID struct {
		Userid string `json:"userid"`
	}

	reqProfileModel struct {
		Userid string `json:"-"`
		Email  string `json:"contact_email"`
		Bio    string `json:"bio"`
	}

	reqProfileImage struct {
		Userid    string `json:"-"`
		Image     io.Reader
		ImageType string
	}

	resProfileUpdate struct {
		Userid string `json:"userid"`
	}

	resProfileModel struct {
		Email string `json:"contact_email"`
		Bio   string `json:"bio"`
		Image string `json:"image"`
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
	return nil
}

func (r *reqProfileImage) valid() *governor.Error {
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	if err := validImage(r.Image); err != nil {
		return err
	}
	if err := validImageType(r.ImageType); err != nil {
		return err
	}
	return nil
}

const (
	moduleID    = "profile"
	imageBucket = "profile-image"
)

type (
	// Profile is a service for storing user profile information
	Profile struct {
		db   *db.Database
		obj  *objstore.Bucket
		gate *gate.Gate
	}
)

// New creates a new Profile service
func New(conf governor.Config, l *logrus.Logger, db *db.Database, obj *objstore.Objstore) *Profile {
	ca := conf.Conf().GetStringMapString("userauth")

	b, err := obj.GetBucketDefLoc(imageBucket)
	if err != nil {
		l.Errorf("failed to get bucket: %s\n", err.Error())
	}

	l.Info("initialized profile service")

	return &Profile{
		db:   db,
		obj:  b,
		gate: gate.New(ca["secret"], ca["issuer"]),
	}
}

// Mount is a collection of routes for accessing and modifying profile data
func (p *Profile) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	r.POST("/:id", func(c echo.Context) error {
		rprofile := &reqProfileModel{}
		if err := c.Bind(rprofile); err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		rprofile.Userid = c.Param("id")
		if err := rprofile.valid(); err != nil {
			return err
		}

		m := &profilemodel.Model{
			Email: rprofile.Email,
			Bio:   rprofile.Bio,
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

		userid, _ := m.IDBase64()

		return c.JSON(http.StatusCreated, &resProfileUpdate{
			Userid: userid,
		})
	}, p.gate.Owner("id"))

	r.PUT("/:id", func(c echo.Context) error {
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
			return err
		}

		m.Email = rprofile.Email
		m.Bio = rprofile.Bio

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.Owner("id"))

	r.PATCH("/:id/image", func(c echo.Context) error {
		file, err := c.FormFile("image")
		if err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		mediaType, _, err := mime.ParseMediaType(file.Header.Get("Content-Type"))
		if err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		image, err := file.Open()
		if err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		defer func(c io.Closer) {
			err := c.Close()
			if err != nil {
				gerr := governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
				l.WithFields(logrus.Fields{
					"origin": gerr.Origin(),
					"source": gerr.Source(),
					"code":   gerr.Code(),
				}).Error(gerr.Message())
			}
		}(image)
		rprofile := &reqProfileImage{
			Userid:    c.Get("userid").(string),
			Image:     image,
			ImageType: mediaType,
		}
		if err := rprofile.valid(); err != nil {
			return err
		}

		if err := p.obj.Put(rprofile.Userid+"-profile", rprofile.ImageType, image); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.Owner("id"))

	r.DELETE("/:id", func(c echo.Context) error {
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
			return err
		}

		if err := m.Delete(db); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.OwnerOrAdmin("id"))

	r.GET("/:id", func(c echo.Context) error {
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
			return err
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

// Setup is run on service setup
func (p *Profile) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if err := profilemodel.Setup(p.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new profile table")
	return nil
}
