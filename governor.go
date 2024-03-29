package governor

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	defaultMaxHeaderSize = 1 << 20 // 1MB
)

//go:embed banner.txt
var banner string

type (
	// Flags are flags for the server cmd
	Flags struct {
		ConfigFile string
		LogPlain   bool
	}

	// Server is a governor server to which services may be registered
	Server struct {
		services []serviceDef
		settings *settings
		log      *klog.LevelLogger
		tracer   Tracer
		i        chi.Router
		initted  bool
		started  bool
		mu       sync.Mutex
	}
)

// New creates a new Server
func New(opts Opts, serverOpts *ServerOpts) *Server {
	if serverOpts == nil {
		serverOpts = &ServerOpts{}
	}
	return &Server{
		settings: newSettings(opts, *serverOpts),
	}
}

// init initializes the config, registered services, and server
func (s *Server) init(ctx context.Context, flags Flags, log klog.Logger) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initted {
		return nil
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "init"))

	if err := s.settings.init(ctx, flags); err != nil {
		return err
	}

	{
		logOpts := []klog.LoggerOpt{
			klog.OptHandler(log.Handler()),
			klog.OptMinLevelStr(s.settings.logger.level),
		}
		if !flags.LogPlain {
			logOpts = append(logOpts,
				klog.OptSubhandler("",
					klog.AGroup("gov",
						klog.AString("appname", s.settings.config.Appname),
						klog.AString("version", s.settings.config.Version.String()),
						klog.AString("hostname", s.settings.config.Hostname),
						klog.AString("instance", s.settings.config.Instance),
					),
				),
			)
		}
		s.log = klog.NewLevelLogger(klog.New(logOpts...))
	}

	s.tracer = &tracer{
		instance: s.settings.config.Instance,
	}

	i := chi.NewRouter()
	s.i = i

	s.log.Info(ctx, "Init server instance")

	i.Use(stripSlashesMiddleware)
	s.log.Info(ctx, "Init strip slashes middleware")

	{
		trustedproxies := make([]netip.Prefix, 0, len(s.settings.middleware.trustedproxies))
		for _, i := range s.settings.middleware.trustedproxies {
			k, err := netip.ParsePrefix(i)
			if err != nil {
				return kerrors.WithMsg(err, "Invalid proxy CIDR")
			}
			trustedproxies = append(trustedproxies, k)
		}
		i.Use(realIPMiddleware(trustedproxies))
		s.log.Info(ctx, "Init real ip middleware", klog.AString("trustedproxies", strings.Join(s.settings.middleware.trustedproxies, ",")))
	}

	i.Use(s.reqLoggerMiddleware)
	s.log.Info(ctx, "Init request logger")

	if len(s.settings.middleware.routerewrite) > 0 {
		k := make([]string, 0, len(s.settings.middleware.routerewrite))
		for _, i := range s.settings.middleware.routerewrite {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.String())
		}
		i.Use(routeRewriteMiddleware(s.settings.middleware.routerewrite))
		s.log.Info(ctx, "Init route rewriter middleware", klog.AString("rules", strings.Join(k, "; ")))
	}

	if len(s.settings.middleware.allowpaths) > 0 {
		k := make([]string, 0, len(s.settings.middleware.allowpaths))
		for _, i := range s.settings.middleware.allowpaths {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.pattern)
		}
		i.Use(corsPathsAllowAllMiddleware(s.settings.middleware.allowpaths))
		s.log.Info(ctx, "Init middleware allow all cors", klog.AString("paths", strings.Join(k, "; ")))
	}
	if len(s.settings.middleware.alloworigins) > 0 {
		i.Use(cors.Handler(cors.Options{
			AllowedOrigins: s.settings.middleware.alloworigins,
			AllowedMethods: []string{
				http.MethodHead,
				http.MethodGet,
				http.MethodPost,
				http.MethodPut,
				http.MethodPatch,
				http.MethodDelete,
			},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
		s.log.Info(ctx, "Init middleware CORS", klog.AString("alloworigins", strings.Join(s.settings.middleware.alloworigins, ", ")))
	}

	i.Use(s.bodyLimitMiddleware())
	s.log.Info(ctx, "Init middleware body limit", klog.AInt("maxreqsize", s.settings.httpServer.maxReqSize))

	i.Use(s.compressorMiddleware)
	s.log.Info(ctx, "Init middleware compressor")

	i.Use(s.recovererMiddleware)
	s.log.Info(ctx, "Init middleware recoverer")

	s.log.Info(ctx, "Secrets source", klog.AString("source", s.settings.vault.Info()))

	s.initHealth(s.router(s.settings.config.BasePath+"/healthz", s.log.Logger))
	s.log.Info(ctx, "Init health routes")

	if err := s.initServices(ctx); err != nil {
		return err
	}

	s.initted = true
	return nil
}

// Start runs init and starts registered services
func (s *Server) Start(ctx context.Context, flags Flags, log klog.Logger) error {
	if err := s.init(ctx, flags, log); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.started {
		return nil
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "start"))

	if err := s.startServices(ctx); err != nil {
		return err
	}

	s.started = true
	return nil
}

// Stop stops all services
func (s *Server) Stop(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initted {
		return
	}
	s.stopServices(ctx)
	s.initted = false
	s.started = false
}

// ServeHTTP implements [net/http.Handler]
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.i.ServeHTTP(w, r)
}

// Serve starts the registered services and the server
func (s *Server) Serve(ctx context.Context, flags Flags, log klog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := s.Start(ctx, flags, log); err != nil {
		return err
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "start"))
	s.log.Info(ctx, "Init http server with configuration",
		klog.AString("addr", s.settings.config.Addr),
		klog.AInt("maxheadersize", s.settings.httpServer.maxHeaderSize),
		klog.AString("maxconnread", s.settings.httpServer.maxConnRead.String()),
		klog.AString("maxconnheader", s.settings.httpServer.maxConnHeader.String()),
		klog.AString("maxconnwrite", s.settings.httpServer.maxConnWrite.String()),
		klog.AString("maxconnidle", s.settings.httpServer.maxConnIdle.String()),
	)
	srv := http.Server{
		Addr:              s.settings.config.Addr,
		Handler:           s,
		ReadTimeout:       s.settings.httpServer.maxConnRead,
		ReadHeaderTimeout: s.settings.httpServer.maxConnHeader,
		WriteTimeout:      s.settings.httpServer.maxConnWrite,
		IdleTimeout:       s.settings.httpServer.maxConnIdle,
		MaxHeaderBytes:    s.settings.httpServer.maxHeaderSize,
	}
	if s.settings.showBanner {
		fmt.Printf("%s\n",
			fmt.Sprintf(banner, s.settings.config.Version.Num),
		)
	}
	s.log.Info(ctx, "Starting server")
	go func() {
		defer cancel()
		if err := srv.ListenAndServe(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Shutting down server"))
		}
	}()
	<-ctx.Done()
	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "stop"))
	s.log.Info(ctx, "Shutdown process begin")
	shutdownCtx, shutdownCancel := context.WithTimeout(klog.ExtendCtx(context.Background(), ctx), 16*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.log.Err(shutdownCtx, kerrors.WithMsg(err, "Shutdown server error"))
	}
	s.Stop(shutdownCtx)
	return nil
}

type (
	// ReqSetup is a service setup request
	ReqSetup struct{}
)

// Setup runs init and setup
func (s *Server) Setup(ctx context.Context, flags Flags, log klog.Logger) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := s.init(ctx, flags, log); err != nil {
		return err
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "setup"))
	if err := s.setupServices(ctx, ReqSetup{}); err != nil {
		return err
	}
	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "stop"))
	s.log.Info(ctx, "Shutdown process begin")
	shutdownCtx, shutdownCancel := context.WithTimeout(klog.ExtendCtx(context.Background(), ctx), 16*time.Second)
	defer shutdownCancel()
	s.Stop(shutdownCtx)
	return nil
}
