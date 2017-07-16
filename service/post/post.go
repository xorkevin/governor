package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

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

const (
	moduleIDPost = moduleID + ".post"
)

// Mount is a collection of routes for accessing and modifying post data
func (p *Post) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	if err := p.mountRest(conf, r.Group("/p"), l); err != nil {
		return err
	}
	if err := p.mountGroup(conf, r.Group("/g"), l); err != nil {
		return err
	}

	l.Info("mounted post service")

	return nil
}

// Health is a check for service health
func (p *Post) Health() *governor.Error {
	return nil
}
