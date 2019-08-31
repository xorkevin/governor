package governor

import (
	"fmt"
	"github.com/labstack/echo"
	"net/http"
)

type (
	// Service is an interface for services
	Service interface {
		Register(r ConfigRegistrar)
		Init(c Config, l Logger, r *echo.Group) error
		Setup(rsetup ReqSetupPost) error
		Health() error
		Start() error
	}

	serviceDef struct {
		name string
		url  string
		r    Service
	}
)

// Register adds the service to the governor Server and runs service.Register
func (s *Server) Register(name string, url string, r Service) {
	s.services = append(s.services, serviceDef{
		name: name,
		url:  url,
		r:    r,
	})
	r.Register(s.config.registrar(name))
}

func (s *Server) setupServices(rsetup ReqSetupPost) error {
	if s.setupRun {
		s.logger.Warn("govsetup: setup already run", nil)
		return NewErrorUser("setup already run", http.StatusForbidden, nil)
	}
	s.logger.Info("setup all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Setup(rsetup); err != nil {
			s.logger.Error(fmt.Sprintf("setup service %s failed", i.name), map[string]string{
				"setup": i.name,
				"error": err.Error(),
			})
			return err
		}
		s.logger.Info(fmt.Sprintf("setup service %s", i.name), map[string]string{
			"setup": i.name,
		})
	}
	s.logger.Info("setup all services complete", nil)
	return nil
}

func (s *Server) checkHealthServices() []error {
	k := []error{}
	for _, i := range s.services {
		if err := i.r.Health(); err != nil {
			k = append(k, err)
		}
	}
	return k
}

func (s *Server) initServices() error {
	s.logger.Info("init all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Init(s.config, s.logger, s.i.Group(s.config.BaseURL+i.url)); err != nil {
			s.logger.Error(fmt.Sprintf("init service %s failed", i.name), map[string]string{
				"init":  i.name,
				"error": err.Error(),
			})
			return err
		}
		s.logger.Info(fmt.Sprintf("init service %s", i.name), map[string]string{
			"init": i.name,
		})
	}
	s.logger.Info("init all services complete", nil)
	return nil
}

func (s *Server) startServices() error {
	s.logger.Info("start all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Start(); err != nil {
			s.logger.Error(fmt.Sprintf("start service %s failed", i.name), map[string]string{
				"start": i.name,
				"error": err.Error(),
			})
			return err
		}
		s.logger.Info(fmt.Sprintf("start service %s", i.name), map[string]string{
			"start": i.name,
		})
	}
	s.logger.Info("start all services complete", nil)
	return nil
}
