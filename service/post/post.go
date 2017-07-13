package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	reqPostPost struct {
		Postid  string `json:"postid"`
		Userid  string `json:"userid"`
		Tags    string `json:"group_tags"`
		Content string `json:"content"`
	}

	resPost struct {
		Postid       string `json:"postid"`
		Userid       string `json:"userid"`
		Tags         string `json:"group_tags"`
		Content      string `json:"content"`
		CreationTime int64  `json:"creation_time"`
	}
)

func (r *reqPostPost) valid() *governor.Error {
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
	r.POST("/", func(c echo.Context) error {
		return c.NoContent(http.StatusNoContent)
	}, p.gate.User())

	r.GET("/:id", func(c echo.Context) error {
		return c.JSON(http.StatusOK, &resPost{})
	})

	l.Info("mounted post service")

	return nil
}

// Health is a check for service health
func (p *Post) Health() *governor.Error {
	return nil
}
