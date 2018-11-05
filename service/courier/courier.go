package courier

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/courier/model"
	"github.com/hackform/governor/service/user/gate"
	"github.com/labstack/echo"
)

const (
	moduleID = "courier"
	min1     = 60
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
		repo         couriermodel.Repo
		cache        cache.Cache
		gate         gate.Gate
		fallbackLink string
	}

	courierRouter struct {
		service courierService
	}
)

// New creates a new Courier service
func New(conf governor.Config, l governor.Logger, repo couriermodel.Repo, ch cache.Cache, g gate.Gate) Service {
	c := conf.Conf().GetStringMapString("courier")
	fallbackLink := c["fallback_link"]
	if len(fallbackLink) == 0 {
		l.Warn("courier: fallback link is not set", moduleID, "fallback_link unset", 0, nil)
	} else if err := validURL(fallbackLink); err != nil {
		l.Warn(err.Message(), moduleID, "invalid fallback_link", 0, nil)
	}
	l.Info("initialized courier service", moduleID, "initialize courier service", 0, map[string]string{
		"fallback_link": fallbackLink,
	})
	return &courierService{
		repo:         repo,
		cache:        ch,
		gate:         g,
		fallbackLink: fallbackLink,
	}
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
