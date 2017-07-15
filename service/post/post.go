package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/post/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type (
	reqPostPost struct {
		Userid  string `json:"userid"`
		Tag     string `json:"group_tag"`
		Content string `json:"content"`
	}

	reqPostPut struct {
		Postid  string `json:"postid"`
		Content string `json:"content"`
	}

	reqPostGet struct {
		Postid string `json:"postid"`
	}

	resPost struct {
		Postid       []byte `json:"postid"`
		Userid       []byte `json:"userid"`
		Tag          string `json:"group_tag"`
		Content      string `json:"content"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (r *reqPostPost) valid() *governor.Error {
	if err := hasUserid(r.Userid); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	if err := validGroup(r.Tag); err != nil {
		return err
	}
	return nil
}

func (r *reqPostPut) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	if err := validContent(r.Content); err != nil {
		return err
	}
	return nil
}

func (r *reqPostGet) valid() *governor.Error {
	if err := hasPostid(r.Postid); err != nil {
		return err
	}
	return nil
}

const (
	moduleID = "post"
)

type (
	// Post is a service for creating posts
	Post struct {
		db    *db.Database
		cache *cache.Cache
		gate  *gate.Gate
	}
)

// New creates a new Post service
func New(conf governor.Config, l *logrus.Logger, db *db.Database, ch *cache.Cache) *Post {
	ca := conf.Conf().GetStringMapString("userauth")

	l.Info("initialized post service")

	return &Post{
		db:    db,
		cache: ch,
		gate:  gate.New(ca["secret"], ca["issuer"]),
	}
}

// Mount is a collection of routes for accessing and modifying post data
func (p *Post) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := p.db.DB()

	r.POST("/", func(c echo.Context) error {
		rpost := &reqPostPost{}
		if err := c.Bind(rpost); err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		if err := rpost.valid(); err != nil {
			return err
		}

		m, err := postmodel.New(rpost.Userid, rpost.Tag, rpost.Content)
		if err != nil {
			err.AddTrace(moduleID)
			return err
		}

		if err := m.Insert(db); err != nil {
			if err.Code() == 3 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleID)
			return err
		}

		t, _ := time.Now().MarshalText()
		postid, _ := m.IDBase64()
		userid, _ := m.UserIDBase64()
		l.WithFields(logrus.Fields{
			"time":   string(t),
			"origin": moduleID,
			"postid": postid,
			"userid": userid,
		}).Info("post created")

		return c.NoContent(http.StatusNoContent)
	}, p.gate.User())

	r.PUT("/:id", func(c echo.Context) error {
		rpost := &reqPostPut{}
		if err := c.Bind(rpost); err != nil {
			return governor.NewErrorUser(moduleID, err.Error(), 0, http.StatusBadRequest)
		}
		rpost.Postid = c.Param("id")
		if err := rpost.valid(); err != nil {
			return err
		}

		m, err := postmodel.GetByIDB64(db, rpost.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			return err
		}

		m.Content = rpost.Content

		if err := m.Update(db); err != nil {
			err.AddTrace(moduleID)
			return err
		}

		return c.NoContent(http.StatusNoContent)
	}, p.gate.User())

	r.GET("/:id", func(c echo.Context) error {
		rpost := &reqPostGet{
			Postid: c.Param("id"),
		}
		if err := rpost.valid(); err != nil {
			return err
		}

		m, err := postmodel.GetByIDB64(db, rpost.Postid)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			return err
		}

		return c.JSON(http.StatusOK, &resPost{
			Postid:       m.Postid,
			Userid:       m.Userid,
			Tag:          m.Tag,
			Content:      m.Content,
			CreationTime: m.CreationTime,
		})
	})

	l.Info("mounted post service")

	return nil
}

// Health is a check for service health
func (p *Post) Health() *governor.Error {
	return nil
}
