package governor

import (
	"context"
	"crypto/subtle"
	"net/http"

	"xorkevin.dev/governor/service/state"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
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
		Init(ctx context.Context, c Config, r ConfigReader, l klog.Logger, m Router) error
		Setup(ctx context.Context, req ReqSetup) error
		PostSetup(ctx context.Context, req ReqSetup) error
		Start(ctx context.Context) error
		Stop(ctx context.Context)
		Health(ctx context.Context) error
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

func (s *Server) hasFirstSetupRun() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.firstSetupRun
}

func (s *Server) setFirstSetupRun(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.firstSetupRun {
		return
	}
	s.firstSetupRun = v
}

func (s *Server) setupServices(ctx context.Context, rsetup ReqSetup) error {
	if !s.hasFirstSetupRun() {
		m, err := s.state.Get(ctx)
		if err != nil {
			return ErrWithRes(err, http.StatusInternalServerError, "", "Failed to get state")
		}
		s.setFirstSetupRun(m.Setup)
	}
	if err := rsetup.valid(); err != nil {
		return err
	}
	if s.hasFirstSetupRun() {
		if rsetup.First {
			return ErrWithRes(nil, http.StatusBadRequest, "", "First setup already run")
		}
		var secret secretSetup
		if err := s.config.getSecret(ctx, "setupsecret", 0, &secret); err != nil {
			return kerrors.WithMsg(err, "Invalid setup secret")
		}
		if subtle.ConstantTimeCompare([]byte(rsetup.Secret), []byte(secret.Secret)) != 1 {
			return ErrWithRes(nil, http.StatusForbidden, "", "Invalid setup secret")
		}
	} else {
		if !rsetup.First {
			return ErrWithRes(nil, http.StatusBadRequest, "", "First setup not yet run")
		}
	}
	rsetup.Secret = ""

	// To avoid partial setup, no request context is passed beyond this point

	ctx = klog.ExtendCtx(context.Background(), ctx, nil)

	s.log.Info(ctx, "Setup all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Setup(ctx, rsetup); err != nil {
			err := kerrors.WithMsg(err, "Setup service failed")
			s.log.Err(ctx, err, klog.Fields{
				"gov.service": i.name,
			})
			return err
		}
		s.log.Info(ctx, "Setup service success", klog.Fields{
			"gov.service": i.name,
		})
	}

	s.log.Info(ctx, "Running postsetup for all services", nil)
	for _, i := range s.services {
		if err := i.r.PostSetup(ctx, rsetup); err != nil {
			err := kerrors.WithMsg(err, "Post setup service failed")
			s.log.Err(ctx, err, klog.Fields{
				"gov.service": i.name,
			})
			return err
		}
		s.log.Info(ctx, "Post setup service success", klog.Fields{
			"gov.service": i.name,
		})
	}

	if rsetup.First {
		if err := s.state.Setup(context.Background(), state.ReqSetup{
			Version: s.config.version.Num,
			VHash:   s.config.version.Hash,
		}); err != nil {
			err := kerrors.WithMsg(err, "Setup state service failed")
			s.log.Err(ctx, err, nil)
			return ErrWithRes(err, http.StatusInternalServerError, "", "Failed to set state")
		}
		s.setFirstSetupRun(true)
	}
	s.log.Info(ctx, "Setup all services complete", nil)
	return nil
}

func (s *Server) checkHealthServices(ctx context.Context) []error {
	var k []error
	for _, i := range s.services {
		if err := i.r.Health(ctx); err != nil {
			k = append(k, err)
		}
	}
	return k
}

func (s *Server) initServices(ctx context.Context) error {
	s.log.Info(ctx, "Init all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Init(ctx, *s.config, s.config.reader(i.serviceOpt), s.log.Logger.Sublogger(i.name, klog.Fields{"gov.service": i.name}), s.router(s.config.BaseURL+i.url)); err != nil {
			err := kerrors.WithMsg(err, "Init service failed")
			s.log.Err(ctx, err, klog.Fields{
				"gov.service": i.name,
			})
			return err
		}
		s.log.Info(ctx, "Init service success", klog.Fields{
			"gov.service": i.name,
		})
	}
	s.log.Info(ctx, "Init all services complete", nil)
	return nil
}

func (s *Server) startServices(ctx context.Context) error {
	s.log.Info(ctx, "Start all services begin", nil)
	for _, i := range s.services {
		if err := i.r.Start(ctx); err != nil {
			err := kerrors.WithMsg(err, "Start service failed")
			s.log.Err(ctx, err, klog.Fields{
				"gov.service": i.name,
			})
			return err
		}
		s.log.Info(ctx, "Start service success", klog.Fields{
			"gov.service": i.name,
		})
	}
	s.log.Info(ctx, "Start all services complete", nil)
	return nil
}

func (s *Server) stopServices(ctx context.Context) {
	s.log.Info(ctx, "Stop all services begin", nil)
	sl := len(s.services)
	for n := range s.services {
		i := s.services[sl-n-1]
		i.r.Stop(ctx)
		s.log.Info(ctx, "Stop service", klog.Fields{
			"gov.service": i.name,
		})
	}
	s.log.Info(ctx, "Stop all services complete", nil)
}
