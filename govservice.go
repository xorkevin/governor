package governor

import (
	"context"
	"fmt"
	"sync/atomic"

	"xorkevin.dev/governor/util/uid"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

type (
	// Service is an interface for governor services
	//
	// A governor service may be in one of 4 stages in its lifecycle.
	//
	//   1. Register: register the service config
	//   2. Init: read configuration and make service functional
	//   3. Start: start the service
	//   4. Stop: stop the service
	//
	// A setup task runs through the following lifecycle methods
	//
	//   1. Register: register the service config
	//   2. Init: read configuration and make service functional
	//   3. Setup: sets up the service
	//   4. Stop: stop the service
	//
	// Furthermore, a service may be polled for its health via Health.
	Service interface {
		Register(r ConfigRegistrar)
		Init(ctx context.Context, r ConfigReader, kit ServiceKit) error
		Start(ctx context.Context) error
		Stop(ctx context.Context)
		Setup(ctx context.Context, req ReqSetup) error
		Health(ctx context.Context) error
	}

	ServiceKit struct {
		Logger klog.Logger
		Router Router
		Tracer Tracer
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

func (s *Server) setupServices(ctx context.Context, rsetup ReqSetup) error {
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
		if err := i.r.Init(ctx, s.settings.reader(i.opt), ServiceKit{
			Logger: l,
			Router: s.router(s.settings.config.BasePath+i.opt.url, l),
			Tracer: s.tracer,
		}); err != nil {
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

type (
	Tracer interface {
		LReqID() string
	}

	tracer struct {
		instance string
		reqcount atomic.Uint32
	}
)

func (t *tracer) LReqID() string {
	return uid.NewSnowflake(t.reqcount.Add(1)).Base64() + t.instance
}
