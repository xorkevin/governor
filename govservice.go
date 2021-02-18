package governor

import (
	"context"
	"fmt"
	"net/http"

	"xorkevin.dev/governor/service/state"
)

type (
	// Service is an interface for governor services
	//
	// A governor service may be in one of 5 stages in its lifecycle.
	//
	// 1. Register: register the service on the config
	//
	// 2. Init: initialize any connections necessary for the service to function
	//
	// 3. Setup: sets up the service for the first time such as creating database
	// tables and mounting routes
	//
	// 4. Start: start the service
	//
	// 5. Stop: stop the service
	//
	// Register and Init always occur first when a governor application is
	// launched. Then Setup and Start may occur in either order, or not at all.
	// Stop runs when the server begins the shutdown process
	Service interface {
		Register(inj Injector, r ConfigRegistrar, jr JobRegistrar)
		Init(ctx context.Context, c Config, r ConfigReader, l Logger, m Router) error
		Setup(req ReqSetup) error
		Start(ctx context.Context) error
		Stop(ctx context.Context)
		Health() error
	}

	serviceOpt struct {
		name string
		url  string
	}

	serviceDef struct {
		serviceOpt
		r Service
	}
)

// Register adds the service to the governor Server and runs service.Register
func (s *Server) Register(name string, url string, r Service) {
	s.services = append(s.services, serviceDef{
		serviceOpt: serviceOpt{
			name: name,
			url:  url,
		},
		r: r,
	})
	r.Register(s.inj, s.config.registrar(name), nil)
}

func (s *Server) setupServices(rsetup ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if s.setupRun {
		l.Warn("setup already run", nil)
		return NewErrorUser("Setup already run", http.StatusForbidden, nil)
	}
	m, err := s.state.Get()
	if err != nil {
		return NewError("Failed to get state", http.StatusInternalServerError, err)
	}
	if m.Setup {
		s.setupRun = true
		l.Warn("setup already run", nil)
		return NewErrorUser("Setup already run", http.StatusForbidden, nil)
	}
	if err := rsetup.valid(); err != nil {
		return err
	}

	l.Info("setup all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Setup(rsetup); err != nil {
			l.Error(fmt.Sprintf("setup service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("setup service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	if err := s.state.Setup(state.ReqSetup{
		Version: s.config.version.Num,
		VHash:   s.config.version.Hash,
	}); err != nil {
		l.Error("setup state service failed", map[string]string{
			"error": err.Error(),
		})
		return NewError("Failed to set state", http.StatusInternalServerError, err)
	}
	s.setupRun = true
	l.Info("setup all services complete", nil)
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

func (s *Server) initServices(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "init",
	})
	l.Info("init all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Init(ctx, *s.config, s.config.reader(i.serviceOpt), s.logger.Subtree(i.name), s.router(s.config.BaseURL+i.url)); err != nil {
			l.Error(fmt.Sprintf("init service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("init service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	l.Info("init all services complete", nil)
	return nil
}

func (s *Server) startServices(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})
	l.Info("start all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Start(ctx); err != nil {
			l.Error(fmt.Sprintf("start service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("start service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	l.Info("start all services complete", nil)
	return nil
}

func (s *Server) stopServices(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	l.Info("stop all services begin", nil)
	sl := len(s.services)
	for n := range s.services {
		i := s.services[sl-n-1]
		i.r.Stop(ctx)
		l.Info(fmt.Sprintf("stop service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	l.Info("stop all services complete", nil)
}
