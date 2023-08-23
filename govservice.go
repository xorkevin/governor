package governor

import (
	"context"
	"crypto/subtle"
	"fmt"
	"net/http"

	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Service is an interface for governor services
	//
	// A governor service may be in one of 4 stages in its lifecycle.
	//
	//   1. Register: register the service config
	//   2. Init: initialize any connections necessary for the service to
	//      function
	//   3. Start: start the service
	//   4. Stop: stop the service
	//
	// Furthermore, a service may be polled for its health via Health.
	//
	// Setup sets up the service for the first time such as creating database
	// tables and mounting routes
	//
	// Register, Init, and Start are run when a governor application is launched.
	// Then Setup will run when setup is triggered. Stop runs when the server
	// begins the shutdown process.
	Service interface {
		Register(r ConfigRegistrar)
		Init(ctx context.Context, r ConfigReader, l klog.Logger, m Router) error
		Start(ctx context.Context) error
		Stop(ctx context.Context)
		Setup(ctx context.Context, req ReqSetup) error
		Health(ctx context.Context) error
	}

	serviceOpt struct {
		name string
		url  string
	}

	serviceDef struct {
		opt serviceOpt
		r   Service
	}
)

// Register adds the service to the governor Server and runs service.Register
func (s *Server) Register(name string, url string, r Service) {
	s.services = append(s.services, serviceDef{
		opt: serviceOpt{
			name: name,
			url:  url,
		},
		r: r,
	})
	r.Register(s.settings.registrar(name))
}

type (
	secretSetup struct {
		Secret string `mapstructure:"secret"`
	}
)

func (s *Server) setupServices(ctx context.Context, reqsecret string, rsetup ReqSetup) error {
	var secret secretSetup
	if err := s.settings.getSecret(ctx, "setupsecret", 60, &secret); err != nil {
		return kerrors.WithMsg(err, "Invalid setup secret")
	}
	if subtle.ConstantTimeCompare([]byte(reqsecret), []byte(secret.Secret)) != 1 {
		return ErrWithRes(nil, http.StatusForbidden, "", "Invalid setup secret")
	}

	// To avoid partial setup, no request context is passed beyond this point

	ctx = klog.ExtendCtx(context.Background(), ctx)

	s.log.Info(ctx, "Setup all services begin")
	for _, i := range s.services {
		if err := i.r.Setup(ctx, rsetup); err != nil {
			err := kerrors.WithMsg(err, "Setup service failed")
			s.log.Err(ctx, err, klog.AString("service", i.opt.name))
			return err
		}
		s.log.Info(ctx, "Setup service success", klog.AString("service", i.opt.name))
	}

	s.log.Info(ctx, "Setup all services complete")
	return nil
}

func (s *Server) checkHealthServices(ctx context.Context) []error {
	var k []error
	for _, i := range s.services {
		if err := i.r.Health(ctx); err != nil {
			k = append(k, kerrors.WithMsg(err, fmt.Sprintf("Failed healthcheck for service %s", i.opt.name)))
		}
	}
	return k
}

func (s *Server) initServices(ctx context.Context) error {
	s.log.Info(ctx, "Init all services begin")
	for _, i := range s.services {
		l := s.log.Logger.Sublogger(i.opt.name, klog.AString("gov.service", i.opt.name))
		if err := i.r.Init(ctx, s.settings.reader(i.opt), l, s.router(s.settings.config.BasePath+i.opt.url, l)); err != nil {
			err := kerrors.WithMsg(err, "Init service failed")
			s.log.Err(ctx, err, klog.AString("service", i.opt.name))
			return err
		}
		s.log.Info(ctx, "Init service success", klog.AString("service", i.opt.name))
	}
	s.log.Info(ctx, "Init all services complete")
	return nil
}

func (s *Server) startServices(ctx context.Context) error {
	s.log.Info(ctx, "Start all services begin")
	for _, i := range s.services {
		if err := i.r.Start(ctx); err != nil {
			err := kerrors.WithMsg(err, "Start service failed")
			s.log.Err(ctx, err, klog.AString("service", i.opt.name))
			return err
		}
		s.log.Info(ctx, "Start service success", klog.AString("service", i.opt.name))
	}
	s.log.Info(ctx, "Start all services complete")
	return nil
}

func (s *Server) stopServices(ctx context.Context) {
	s.log.Info(ctx, "Stop all services begin")
	sl := len(s.services)
	for n := range s.services {
		i := s.services[sl-n-1]
		i.r.Stop(ctx)
		s.log.Info(ctx, "Stop service", klog.AString("service", i.opt.name))
	}
	s.log.Info(ctx, "Stop all services complete")
}
