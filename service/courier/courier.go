package courier

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/barcode"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/courier/model"
	"github.com/hackform/governor/service/objstore"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
	"strconv"
	"time"
)

const (
	linkImageBucketID       = "courier-link-image"
	min1              int64 = 60
	b1                      = 1000000000
	min15                   = 900
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
		cc              cachecontrol.CacheControl
		fallbackLink    string
		linkPrefix      string
		cacheTime       int64
	}

	courierRouter struct {
		service courierService
	}
)

// New creates a new Courier service
func New(conf governor.Config, l governor.Logger, repo couriermodel.Repo, store objstore.Objstore, code barcode.Generator, ch cache.Cache, g gate.Gate, cc cachecontrol.CacheControl) (Service, error) {
	c := conf.Conf().GetStringMapString("courier")
	fallbackLink := c["fallback_link"]
	linkPrefix := c["link_prefix"]
	cacheTime := min1
	if duration, err := time.ParseDuration(c["cache_time"]); err == nil {
		cacheTime = duration.Nanoseconds() / b1
	} else {
		l.Warn(fmt.Sprintf("courier: fail parse cache duration: %s", c["cache_time"]), nil)
	}
	if len(fallbackLink) == 0 {
		l.Warn("courier: fallback_link is not set", nil)
	} else if err := validURL(fallbackLink); err != nil {
		l.Warn("invalid fallback_link", map[string]string{
			"err": err.Error(),
		})
	}
	if len(linkPrefix) == 0 {
		l.Warn("courier: link_prefix is not set", nil)
	} else if err := validURL(linkPrefix); err != nil {
		l.Warn("invalid link_prefix", map[string]string{
			"err": err.Error(),
		})
	}

	linkImageBucket, err := store.GetBucketDefLoc(linkImageBucketID)
	if err != nil {
		l.Error("fail get image bucket", map[string]string{
			"err": err.Error(),
		})
		return nil, err
	}

	l.Info("initialize courier service", map[string]string{
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
		cc:              cc,
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
	l.Info("mount courier service", nil)
	return nil
}

// Health is a check for service health
func (c *courierService) Health() error {
	return nil
}

// Setup is run on service setup
func (c *courierService) Setup(conf governor.Config, l governor.Logger, rsetup governor.ReqSetupPost) error {
	if err := c.repo.Setup(); err != nil {
		return err
	}
	l.Info("create courierlinks table", nil)
	return nil
}
