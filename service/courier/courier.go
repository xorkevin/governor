package courier

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/barcode"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/courier/model"
	"github.com/hackform/governor/service/objstore"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"strconv"
	"time"
)

const (
	moduleID                = "courier"
	linkImageBucketID       = "courier-link-image"
	min1              int64 = 60
	b1                      = 1000000000
)

type (
	// Courier is a service for sharing information
	Courier interface {
	}

	// Service is the public interface for the courier service server
	Service interface {
		governor.Service
		Courier
	}

	courierService struct {
		logger          governor.Logger
		repo            couriermodel.Repo
		linkImageBucket objstore.Bucket
		barcode         barcode.Generator
		cache           cache.Cache
		gate            gate.Gate
		fallbackLink    string
		linkPrefix      string
		cacheTime       int64
	}

	courierRouter struct {
		service courierService
	}
)

// New creates a new Courier service
func New(conf governor.Config, l governor.Logger, repo couriermodel.Repo, store objstore.Objstore, code barcode.Generator, ch cache.Cache, g gate.Gate) (Service, error) {
	c := conf.Conf().GetStringMapString("courier")
	fallbackLink := c["fallback_link"]
	linkPrefix := c["link_prefix"]
	cacheTime := min1
	if duration, err := time.ParseDuration(c["cache_time"]); err == nil {
		cacheTime = duration.Nanoseconds() / b1
	} else {
		l.Warn(fmt.Sprintf("failed to parse duration: %s", c["cache_time"]), moduleID, "fail parse cache duration", 0, nil)
	}
	if len(fallbackLink) == 0 {
		l.Warn("courier: fallback link is not set", moduleID, "fallback_link unset", 0, nil)
	} else if err := validURL(fallbackLink); err != nil {
		l.Warn(err.Message(), moduleID, "invalid fallback_link", 0, nil)
	}
	if len(linkPrefix) == 0 {
		l.Warn("courier: link prefix is not set", moduleID, "link_prefix unset", 0, nil)
	} else if err := validURL(linkPrefix); err != nil {
		l.Warn(err.Message(), moduleID, "invalid link_prefix", 0, nil)
	}

	linkImageBucket, err := store.GetBucketDefLoc(linkImageBucketID)
	if err != nil {
		err.AddTrace(moduleID)
		return nil, err
	}

	l.Info("initialized courier service", moduleID, "initialize courier service", 0, map[string]string{
		"fallback_link": fallbackLink,
		"link_prefix":   linkPrefix,
		"cache_time":    strconv.FormatInt(cacheTime, 10),
	})
	return &courierService{
		logger:          l,
		repo:            repo,
		linkImageBucket: linkImageBucket,
		barcode:         code,
		cache:           ch,
		gate:            g,
		fallbackLink:    fallbackLink,
		linkPrefix:      linkPrefix,
		cacheTime:       cacheTime,
	}, nil
}

func (c *courierService) newRouter() *courierRouter {
	return &courierRouter{
		service: *c,
	}
}

// Mount is a collection of routes for accessing and modifying courier data
func (c *courierService) Mount(conf governor.Config, l governor.Logger, r *echo.Group) error {
	cr := c.newRouter()
	if err := cr.mountRoutes(conf, r); err != nil {
		return err
	}
	l.Info("mounted courier service", moduleID, "mount courier service", 0, nil)
	return nil
}

// Health is a check for service health
func (c *courierService) Health() *governor.Error {
	return nil
}

// Setup is run on service setup
func (c *courierService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) *governor.Error {
	if err := c.repo.Setup(); err != nil {
		err.AddTrace(moduleID)
		return err
	}
	l.Info("created new courier links table", moduleID, "create courierlinks table", 0, nil)
	return nil
}
