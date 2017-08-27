package profile

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/image"
	"github.com/hackform/governor/service/objstore"
	"github.com/hackform/governor/service/profile/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"io"
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

const (
	moduleID    = "profile"
	imageBucket = "profile-image"
	hour6       = 21600
)

type (
	// Profile is a service for storing user profile information
	Profile interface {
		governor.Service
	}

	profileService struct {
		db   db.Database
		obj  objstore.Bucket
		gate gate.Gate
		img  image.Image
		cc   cachecontrol.CacheControl
	}
)

// New creates a new Profile service
func New(conf governor.Config, l *logrus.Logger, db db.Database, obj objstore.Objstore, g gate.Gate, img image.Image, cc cachecontrol.CacheControl) Profile {
	b, err := obj.GetBucketDefLoc(imageBucket)
	if err != nil {
		l.Errorf("failed to get bucket: %s\n", err.Error())
	}

	l.Info("initialized profile service")

	return &profileService{
		db:   db,
		obj:  b,
		gate: g,
		img:  img,
		cc:   cc,
	}
}

// Mount is a collection of routes for accessing and modifying profile data
func (p *profileService) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	r.POST("/:id", func(c echo.Context) error {
		rprofile := reqProfileModel{}
		if err := c.Bind(&rprofile); err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		rprofile.Userid = c.Param("id")
		if err := rprofile.valid(); err != nil {
			return err
		}

		m := profilemodel.Model{
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

		return c.JSON(http.StatusCreated, resProfileUpdate{
			Userid: userid,
		})
	}, gate.Owner(p.gate, "id"))

	r.PUT("/:id", func(c echo.Context) error {
		rprofile := reqProfileModel{}
		if err := c.Bind(&rprofile); err != nil {
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
	}, gate.Owner(p.gate, "id"))

	r.POST("/:id/image", func(c echo.Context) error {

		img, _ := c.Get("image").(io.Reader)
		img64, _ := c.Get("imageb64").(string)
		userid := c.Param("id")

		m, err := profilemodel.GetByIDB64(db, userid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
			return err
		}

		if err := p.obj.Put(userid+"-profile", image.MediaTypeJpeg, img); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		m.Image = img64
		if err := m.Update(db); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, gate.Owner(p.gate, "id"), p.img.LoadJpeg("image", 200000, 50, "image", "imageb64"))

	r.DELETE("/:id", func(c echo.Context) error {
		rprofile := reqProfileGetID{
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
	}, gate.OwnerOrAdmin(p.gate, "id"))

	r.GET("/:id", func(c echo.Context) error {
		rprofile := reqProfileGetID{
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

		return c.JSON(http.StatusOK, resProfileModel{
			Email: m.Email,
			Bio:   m.Bio,
			Image: m.Image,
		})
	}, p.cc.Control(true, false, hour6, func(c echo.Context) (string, *governor.Error) {
		return "", nil
	}))

	r.GET("/:id/image", func(c echo.Context) error {
		rprofile := reqProfileGetID{
			Userid: c.Param("id"),
		}
		if err := rprofile.valid(); err != nil {
			return err
		}

		obj, objinfo, err := p.obj.Get(rprofile.Userid + "-profile")
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
			return err
		}
		return c.Stream(http.StatusOK, objinfo.ContentType, obj)
	}, p.cc.Control(true, false, hour6, func(c echo.Context) (string, *governor.Error) {
		rprofile := reqProfileGetID{
			Userid: c.Param("id"),
		}
		if err := rprofile.valid(); err != nil {
			return "", err
		}

		objinfo, err := p.obj.Stat(rprofile.Userid + "-profile")
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
			return "", err
		}

		return objinfo.ETag, nil
	}))

	l.Info("mounted profile service")

	return nil
}

// Health is a check for service health
func (p *profileService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (p *profileService) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if err := profilemodel.Setup(p.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new profile table")
	return nil
}
