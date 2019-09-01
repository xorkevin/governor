package governor

import (
	"context"
	"fmt"
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor/service/state"
)

type (
	// Service is an interface for services
	Service interface {
		Register(r ConfigRegistrar)
		Init(c Config, l Logger, r *echo.Group) error
		Setup(rsetup ReqSetup) error
		Health() error
		Start(ctx context.Context) error
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

func (s *Server) setupServices(rsetup ReqSetup) error {
	if s.setupRun {
		s.logger.Warn("govsetup: setup already run", nil)
		return NewErrorUser("setup already run", http.StatusForbidden, nil)
	}
	m, err := s.state.Get()
	if err != nil {
		return err
	}
	if m.Setup {
		s.setupRun = true
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
	if err := s.state.Setup(state.ReqSetup{
		Orgname: rsetup.Orgname,
	}); err != nil {
		s.logger.Error("setup state service failed", map[string]string{
			"error": err.Error(),
		})
		return err
	}
	s.setupRun = true
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

func (s *Server) startServices(ctx context.Context) error {
	s.logger.Info("start all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Start(ctx); err != nil {
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
