package governor

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
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

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "init",
	})

	if err := s.settings.init(ctx, s.flags); err != nil {
		return err
	}
	s.log = newLogger(s.settings.config, s.settings.logger)

	i := chi.NewRouter()
	s.i = i

	s.log.Info(ctx, "Init server instance", nil)

	i.Use(stripSlashesMiddleware)
	s.log.Info(ctx, "Init strip slashes middleware", nil)

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
		s.log.Info(ctx, "Init real ip middleware", klog.Fields{
			"realip.trustedproxies": strings.Join(s.settings.middleware.trustedproxies, ","),
		})
	}

	i.Use(s.reqLoggerMiddleware)
	s.log.Info(ctx, "Init request logger", nil)

	if len(s.settings.middleware.routerewrite) > 0 {
		k := make([]string, 0, len(s.settings.middleware.routerewrite))
		for _, i := range s.settings.middleware.routerewrite {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.String())
		}
		i.Use(routeRewriteMiddleware(s.settings.middleware.routerewrite))
		s.log.Info(ctx, "Init route rewriter middleware", klog.Fields{
			"routerrewrite.rules": strings.Join(k, "; "),
		})
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
		s.log.Info(ctx, "Init middleware allow all cors", klog.Fields{
			"cors.paths": strings.Join(k, "; "),
		})
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
		s.log.Info(ctx, "Init middleware CORS", klog.Fields{
			"cors.alloworigins": strings.Join(s.settings.middleware.alloworigins, ", "),
		})
	}

	if limit, err := bytefmt.ToBytes(s.settings.httpServer.maxReqSize); err != nil {
		s.log.Warn(ctx, "Invalid maxreqsize format for middlware body limit", klog.Fields{
			"bodylimit.maxreqsize": s.settings.httpServer.maxReqSize,
		})
	} else {
		i.Use(s.bodyLimitMiddleware(limit))
		s.log.Info(ctx, "Init middleware body limit", klog.Fields{
			"bodylimit.maxreqsize": s.settings.httpServer.maxReqSize,
		})
	}

	i.Use(s.compressorMiddleware)
	s.log.Info(ctx, "Init middleware compressor", nil)

	i.Use(s.recovererMiddleware)
	s.log.Info(ctx, "Init middleware recoverer", nil)

	s.log.Info(ctx, "Secrets source", klog.Fields{
		"gov.secrets.source": s.settings.vault.Info(),
	})

	s.initSetup(s.router(s.settings.config.BasePath+"/setupz", s.log.Logger))
	s.log.Info(ctx, "Init setup routes", nil)
	s.initHealth(s.router(s.settings.config.BasePath+"/healthz", s.log.Logger))
	s.log.Info(ctx, "Init health routes", nil)

	if err := s.initServices(ctx); err != nil {
		return err
	}

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "start",
	})
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
func (s *Server) Serve() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.Init(ctx); err != nil {
		if s.log != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Init failed"), nil)
		}
		return err
	}

	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "start",
	})
	maxHeaderSize := defaultMaxHeaderSize
	if limit, err := bytefmt.ToBytes(s.settings.httpServer.maxHeaderSize); err != nil {
		s.log.Warn(ctx, "Invalid maxheadersize format for http server", klog.Fields{
			"http.server.maxheadersize": s.settings.httpServer.maxReqSize,
		})
	} else {
		maxHeaderSize = int(limit)
	}
	maxConnRead := 5 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnRead); err != nil {
		s.log.Warn(ctx, "Invalid maxconnread time for http server", klog.Fields{
			"http.server.maxconnread": s.settings.httpServer.maxConnRead,
		})
	} else {
		maxConnRead = t
	}
	maxConnHeader := 2 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnHeader); err != nil {
		s.log.Warn(ctx, "Invalid maxconnheader time for http server", klog.Fields{
			"http.server.maxconnheader": s.settings.httpServer.maxConnHeader,
		})
	} else {
		maxConnHeader = t
	}
	maxConnWrite := 5 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnWrite); err != nil {
		s.log.Warn(ctx, "Invalid maxconnwrite time for http server", klog.Fields{
			"http.server.maxconnwrite": s.settings.httpServer.maxConnWrite,
		})
	} else {
		maxConnWrite = t
	}
	maxConnIdle := 5 * time.Second
	if t, err := time.ParseDuration(s.settings.httpServer.maxConnIdle); err != nil {
		s.log.Warn(ctx, "Invalid maxconnidle time for http server", klog.Fields{
			"http.server.maxconnidle": s.settings.httpServer.maxConnIdle,
		})
	} else {
		maxConnIdle = t
	}
	s.log.Info(ctx, "Init http server with configuration", klog.Fields{
		"http.server.addr":          s.settings.config.Addr,
		"http.server.maxheadersize": strconv.Itoa(maxHeaderSize),
		"http.server.maxconnread":   maxConnRead.String(),
		"http.server.maxconnheader": maxConnHeader.String(),
		"http.server.maxconnwrite":  maxConnWrite.String(),
		"http.server.maxconnidle":   maxConnIdle.String(),
	})
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
	s.log.Info(ctx, "Starting server", nil)
	go func() {
		defer cancel()
		if err := srv.ListenAndServe(); err != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Shutting down server"), nil)
		}
	}()
	s.waitForInterrupt(ctx)
	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "stop",
	})
	s.log.Info(ctx, "Shutdown process begin", nil)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(klog.ExtendCtx(context.Background(), ctx, nil), 16*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		s.log.Err(shutdownCtx, kerrors.WithMsg(err, "Shutdown server error"), nil)
	}
	s.Stop(shutdownCtx)
	return nil
}

func (s *Server) waitForInterrupt(ctx context.Context) {
	notifyCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	<-notifyCtx.Done()
}
