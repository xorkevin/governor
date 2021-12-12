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
	// A governor service may be in one of 6 stages in its lifecycle.
	//
	// 1. Register: register the service on the config
	//
	// 2. Init: initialize any connections necessary for the service to function
	//
	// 3. Setup: sets up the service for the first time such as creating database
	// tables and mounting routes
	//
	// 4. PostSetup: runs any remaining setup tasks after all other services have
	// completed Setup.
	//
	// 5. Start: start the service
	//
	// 6. Stop: stop the service
	//
	// Register and Init always occur first when a governor application is
	// launched. Then Setup and PostSetup are run if in setup mode. Otherwise
	// Start, is run. Stop runs when the server begins the shutdown process.
	Service interface {
		Register(name string, inj Injector, r ConfigRegistrar, jr JobRegistrar)
		Init(ctx context.Context, c Config, r ConfigReader, l Logger, m Router) error
		Setup(req ReqSetup) error
		PostSetup(req ReqSetup) error
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
	r.Register(name, s.inj, s.config.registrar(name), nil)
}

type (
	secretSetup struct {
		Secret string `mapstructure:"secret"`
	}
)

func (s *Server) setupServices(rsetup ReqSetup) error {
	l := s.logger.WithData(map[string]string{
		"phase": "setup",
	})
	if !s.firstSetupRun {
		m, err := s.state.Get()
		if err != nil {
			return NewError(ErrOptRes(ErrorRes{
				Status:  http.StatusInternalServerError,
				Message: "Failed to get state",
			}), ErrOptInner(err))
		}
		s.firstSetupRun = m.Setup
	}
	if err := rsetup.valid(); err != nil {
		return err
	}
	if s.firstSetupRun {
		if rsetup.First {
			l.Warn("First setup already run", nil)
			return NewError(ErrOptUser, ErrOptRes(ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "First setup already run",
			}))
		}
		var secret secretSetup
		if err := s.config.getSecret("setupsecret", 0, &secret); err != nil {
			return ErrWithMsg(err, "Invalid setup secret")
		}
		if rsetup.Secret != secret.Secret {
			return NewError(ErrOptUser, ErrOptRes(ErrorRes{
				Status:  http.StatusForbidden,
				Message: "Invalid setup secret",
			}))
		}
	} else {
		if !rsetup.First {
			l.Warn("First setup not yet run", nil)
			return NewError(ErrOptUser, ErrOptRes(ErrorRes{
				Status:  http.StatusBadRequest,
				Message: "First setup not yet run",
			}))
		}
	}
	rsetup.Secret = ""

	l.Info("Setup all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Setup(rsetup); err != nil {
			l.Error(fmt.Sprintf("Setup service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("Setup service %s", i.name), map[string]string{
			"service": i.name,
		})
	}

	l.Info("Running PostSetup for all services", nil)
	for _, i := range s.services {
		if err := i.r.PostSetup(rsetup); err != nil {
			l.Error(fmt.Sprintf("Post setup service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("Done post setup service %s", i.name), map[string]string{
			"service": i.name,
		})
	}

	if rsetup.First {
		if err := s.state.Setup(state.ReqSetup{
			Version: s.config.version.Num,
			VHash:   s.config.version.Hash,
		}); err != nil {
			l.Error("Setup state service failed", map[string]string{
				"error": err.Error(),
			})
			return NewError(ErrOptRes(ErrorRes{
				Status:  http.StatusInternalServerError,
				Message: "Failed to set state",
			}), ErrOptInner(err))
		}
	}
	l.Info("Setup all services complete", nil)
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
	l.Info("Init all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Init(ctx, *s.config, s.config.reader(i.serviceOpt), s.logger.Subtree(i.name), s.router(s.config.BaseURL+i.url)); err != nil {
			l.Error(fmt.Sprintf("Init service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("Init service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	l.Info("Init all services complete", nil)
	return nil
}

func (s *Server) startServices(ctx context.Context) error {
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})
	l.Info("Start all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Start(ctx); err != nil {
			l.Error(fmt.Sprintf("Start service %s failed", i.name), map[string]string{
				"service": i.name,
				"error":   err.Error(),
			})
			return err
		}
		l.Info(fmt.Sprintf("Start service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	l.Info("Start all services complete", nil)
	return nil
}

func (s *Server) stopServices(ctx context.Context) {
	l := s.logger.WithData(map[string]string{
		"phase": "stop",
	})
	l.Info("Stop all services begin", nil)
	sl := len(s.services)
	for n := range s.services {
		i := s.services[sl-n-1]
		i.r.Stop(ctx)
		l.Info(fmt.Sprintf("Stop service %s", i.name), map[string]string{
			"service": i.name,
		})
	}
	l.Info("Stop all services complete", nil)
}
