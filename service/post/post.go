package post

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/post/comment/model"
	"github.com/hackform/governor/service/post/model"
	"github.com/hackform/governor/service/post/vote/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	moduleID = "post"
)

type (
	// Post is a service for creating posts
	Post interface {
		governor.Service
	}

	postService struct {
		db          db.Database
		cache       cache.Cache
		gate        gate.Gate
		cc          cachecontrol.CacheControl
		archiveTime int64
	}
)

const (
	time4month int64 = 9676800
	b1               = 1000000000
	min2             = 120
)

// New creates a new Post service
func New(conf governor.Config, l *logrus.Logger, database db.Database, ch cache.Cache, g gate.Gate, cc cachecontrol.CacheControl) Post {
	cp := conf.Conf().GetStringMapString("post")
	archiveTime := time4month
	if duration, err := time.ParseDuration(cp["archive_time"]); err == nil {
		archiveTime = duration.Nanoseconds() / b1
	}

	l.Infof("post: archive_time: %s", cp["archive_time"])
	l.Info("initialized post service")

	return &postService{
		db:          database,
		cache:       ch,
		gate:        g,
		cc:          cc,
		archiveTime: archiveTime,
	}
}

const (
	moduleIDPost     = moduleID + ".post"
	moduleIDGroup    = moduleID + ".group"
	moduleIDComments = moduleID + ".comments"
)

// Mount is a collection of routes for accessing and modifying post data
func (p *postService) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
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
func (p *postService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (p *postService) Setup(conf governor.Config, l *logrus.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if err := postmodel.Setup(p.db.DB()); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new post table")
	if err := commentmodel.Setup(p.db.DB()); err != nil {
		return err
	}
	l.Info("created new comment table")
	if err := votemodel.Setup(p.db.DB()); err != nil {
		return err
	}
	l.Info("created new vote table")
	return nil
}
