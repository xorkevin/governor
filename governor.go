package governor

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"xorkevin.dev/governor/util/bytefmt"
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
	}

	// Server is a governor server to which services may be registered
	Server struct {
		services []serviceDef
		inj      Injector
		flags    Flags
		settings *settings
		log      *klog.LevelLogger
		i        chi.Router
		running  bool
		mu       *sync.Mutex
	}
)

// New creates a new Server
func New(opts Opts) *Server {
	return &Server{
		services: []serviceDef{},
		inj:      newInjector(context.Background()),
		settings: newSettings(opts),
		mu:       &sync.Mutex{},
	}
}

// SetFlags sets server flags
func (s *Server) SetFlags(flags Flags) {
	s.flags = flags
}

// Init initializes the config, initializes the server and its registered
// services, and starts all services
func (s *Server) Init(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "init"))

	if err := s.settings.init(ctx, s.flags); err != nil {
		return err
	}
	s.log = newLogger(s.settings.config, s.settings.logger)

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

	if limit, err := bytefmt.ToBytes(s.settings.httpServer.maxReqSize); err != nil {
		s.log.Warn(ctx, "Invalid maxreqsize format for middlware body limit", klog.AString("maxreqsize", s.settings.httpServer.maxReqSize))
	} else {
		i.Use(s.bodyLimitMiddleware(limit))
		s.log.Info(ctx, "Init middleware body limit", klog.AString("maxreqsize", s.settings.httpServer.maxReqSize))
	}

	i.Use(s.compressorMiddleware)
	s.log.Info(ctx, "Init middleware compressor")

	i.Use(s.recovererMiddleware)
	s.log.Info(ctx, "Init middleware recoverer")

	s.log.Info(ctx, "Secrets source", klog.AString("source", s.settings.vault.Info()))

	s.initSetup(s.router(s.settings.config.BasePath+"/setupz", s.log.Logger))
	s.log.Info(ctx, "Init setup routes")
	s.initHealth(s.router(s.settings.config.BasePath+"/healthz", s.log.Logger))
	s.log.Info(ctx, "Init health routes")

	if err := s.initServices(ctx); err != nil {
		return err
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "start"))
	if err := s.startServices(ctx); err != nil {
		return err
	}

	s.running = true
	return nil
}

// Stop stops all services
func (s *Server) Stop(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.stopServices(ctx)
	s.running = false
}

// ServeHTTP implements [net/http.Handler]
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.i.ServeHTTP(w, r)
}

// Serve starts the registered services and the server
func (s *Server) Serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := s.Init(ctx); err != nil {
		if s.log != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Init failed"))
		}
		return err
	}

	ctx = klog.CtxWithAttrs(ctx, klog.AString("gov.phase", "start"))
	maxHeaderSize := defaultMaxHeaderSize
	if limit, err := bytefmt.ToBytes(s.settings.httpServer.maxHeaderSize); err != nil {
		s.log.Warn(ctx, "Invalid maxheadersize format for http server", klog.AString("maxheadersize", s.settings.httpServer.maxReqSize))
	} else {
		maxHeaderSize = int(limit)
	}
	maxConnRead := 5 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnRead); err != nil {
		s.log.Warn(ctx, "Invalid maxconnread time for http server", klog.AString("maxconnread", s.settings.httpServer.maxConnRead))
	} else {
		maxConnRead = t
	}
	maxConnHeader := 2 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnHeader); err != nil {
		s.log.Warn(ctx, "Invalid maxconnheader time for http server", klog.AString("maxconnheader", s.settings.httpServer.maxConnHeader))
	} else {
		maxConnHeader = t
	}
	maxConnWrite := 5 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnWrite); err != nil {
		s.log.Warn(ctx, "Invalid maxconnwrite time for http server", klog.AString("maxconnwrite", s.settings.httpServer.maxConnWrite))
	} else {
		maxConnWrite = t
	}
	maxConnIdle := 5 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnIdle); err != nil {
		s.log.Warn(ctx, "Invalid maxconnidle time for http server", klog.AString("maxconnidle", s.settings.httpServer.maxConnIdle))
	} else {
		maxConnIdle = t
	}
	s.log.Info(ctx, "Init http server with configuration",
		klog.AString("addr", s.settings.config.Addr),
		klog.AString("maxheadersize", strconv.Itoa(maxHeaderSize)),
		klog.AString("maxconnread", maxConnRead.String()),
		klog.AString("maxconnheader", maxConnHeader.String()),
		klog.AString("maxconnwrite", maxConnWrite.String()),
		klog.AString("maxconnidle", maxConnIdle.String()),
	)
	srv := http.Server{
		Addr:              s.settings.config.Addr,
		Handler:           s,
		ReadTimeout:       maxConnRead,
		ReadHeaderTimeout: maxConnHeader,
		WriteTimeout:      maxConnWrite,
		IdleTimeout:       maxConnIdle,
		MaxHeaderBytes:    maxHeaderSize,
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
