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
		config   *Config
		log      *klog.LevelLogger
		i        chi.Router
		flags    Flags
	}
)

// New creates a new Server
func New(opts Opts) *Server {
	return &Server{
		services: []serviceDef{},
		inj:      newInjector(context.Background()),
		config:   newConfig(opts),
		flags: Flags{
			ConfigFile: "",
		},
	}
}

// SetFlags sets server flags
func (s *Server) SetFlags(flags Flags) {
	s.flags = flags
}

// init initializes the config, creates a new logger, and initializes the
// server and its registered services
func (s *Server) init(ctx context.Context) error {
	if file := s.flags.ConfigFile; file != "" {
		s.config.setConfigFile(file)
	}
	if err := s.config.init(); err != nil {
		return err
	}
	s.log = newLogger(*s.config)

	i := chi.NewRouter()
	s.i = i

	s.log.Info(ctx, "Init server instance", nil)

	i.Use(stripSlashesMiddleware)
	s.log.Info(ctx, "Init strip slashes middleware", nil)

	if len(s.config.middleware.trustedproxies) > 0 {
		trustedproxies := make([]netip.Prefix, 0, len(s.config.middleware.trustedproxies))
		for _, i := range s.config.middleware.trustedproxies {
			k, err := netip.ParsePrefix(i)
			if err != nil {
				return kerrors.WithMsg(err, "Invalid proxy CIDR")
			}
			trustedproxies = append(trustedproxies, k)
		}
		i.Use(realIPMiddleware(trustedproxies))
		s.log.Info(ctx, "Init real ip middleware", klog.Fields{
			"realip.trustedproxies": strings.Join(s.config.middleware.trustedproxies, ","),
		})
	} else {
		i.Use(realIPMiddleware(nil))
		s.log.Info(ctx, "Init real ip middleware", nil)
	}

	i.Use(s.reqLoggerMiddleware)
	s.log.Info(ctx, "Init request logger", nil)

	if len(s.config.middleware.routerewrite) > 0 {
		k := make([]string, 0, len(s.config.middleware.routerewrite))
		for _, i := range s.config.middleware.routerewrite {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.String())
		}
		i.Use(routeRewriteMiddleware(s.config.middleware.routerewrite))
		s.log.Info(ctx, "Init route rewriter middleware", klog.Fields{
			"routerrewrite.rules": strings.Join(k, "; "),
		})
	}

	if len(s.config.middleware.allowpaths) > 0 {
		k := make([]string, 0, len(s.config.middleware.allowpaths))
		for _, i := range s.config.middleware.allowpaths {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.pattern)
		}
		i.Use(corsPathsAllowAllMiddleware(s.config.middleware.allowpaths))
		s.log.Info(ctx, "Init middleware allow all cors", klog.Fields{
			"cors.paths": strings.Join(k, "; "),
		})
	}
	if len(s.config.middleware.origins) > 0 {
		i.Use(cors.Handler(cors.Options{
			AllowedOrigins: s.config.middleware.origins,
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
			"cors.origins": strings.Join(s.config.middleware.origins, ", "),
		})
	}

	if limit, err := bytefmt.ToBytes(s.config.httpServer.maxReqSize); err != nil {
		s.log.Warn(ctx, "Invalid maxreqsize format for middlware body limit", klog.Fields{
			"bodylimit.maxreqsize": s.config.httpServer.maxReqSize,
		})
	} else {
		i.Use(s.bodyLimitMiddleware(limit))
		s.log.Info(ctx, "Init middleware body limit", klog.Fields{
			"bodylimit.maxreqsize": s.config.httpServer.maxReqSize,
		})
	}

	i.Use(s.compressorMiddleware(s.config.middleware.compressibleTypes, s.config.middleware.preferredEncodings))
	s.log.Info(ctx, "Init middleware compressor", nil)

	i.Use(s.recovererMiddleware)
	s.log.Info(ctx, "Init middleware recoverer", nil)

	s.log.Info(ctx, "Secrets source", klog.Fields{
		"gov.secrets.source": s.config.vault.Info(),
	})

	s.initSetup(s.router(s.config.BaseURL+"/setupz", s.log.Logger))
	s.log.Info(ctx, "Init setup routes", nil)
	s.initHealth(s.router(s.config.BaseURL+"/healthz", s.log.Logger))
	s.log.Info(ctx, "Init health routes", nil)

	if err := s.initServices(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Server) waitForInterrupt(ctx context.Context) {
	notifyCtx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	<-notifyCtx.Done()
}

// Start starts the registered services and the server
func (s *Server) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "init",
	})
	if err := s.init(ctx); err != nil {
		if s.log != nil {
			s.log.Err(ctx, kerrors.WithMsg(err, "Init failed"), nil)
		}
		return err
	}
	ctx = klog.WithFields(ctx, klog.Fields{
		"gov.service.phase": "start",
	})
	if err := s.startServices(ctx); err != nil {
		return err
	}

	maxHeaderSize := defaultMaxHeaderSize
	if limit, err := bytefmt.ToBytes(s.config.httpServer.maxHeaderSize); err != nil {
		s.log.Warn(ctx, "Invalid maxheadersize format for http server", klog.Fields{
			"http.server.maxheadersize": s.config.httpServer.maxReqSize,
		})
	} else {
		maxHeaderSize = int(limit)
	}
	maxConnRead := 5 * time.Second
	if t, err := time.ParseDuration(s.config.httpServer.maxConnRead); err != nil {
		s.log.Warn(ctx, "Invalid maxconnread time for http server", klog.Fields{
			"http.server.maxconnread": s.config.httpServer.maxConnRead,
		})
	} else {
		maxConnRead = t
	}
	maxConnHeader := 2 * time.Second
	if t, err := time.ParseDuration(s.config.httpServer.maxConnHeader); err != nil {
		s.log.Warn(ctx, "Invalid maxconnheader time for http server", klog.Fields{
			"http.server.maxconnheader": s.config.httpServer.maxConnHeader,
		})
	} else {
		maxConnHeader = t
	}
	maxConnWrite := 5 * time.Second
	if t, err := time.ParseDuration(s.config.httpServer.maxConnWrite); err != nil {
		s.log.Warn(ctx, "Invalid maxconnwrite time for http server", klog.Fields{
			"http.server.maxconnwrite": s.config.httpServer.maxConnWrite,
		})
	} else {
		maxConnWrite = t
	}
	maxConnIdle := 5 * time.Second
	if t, err := time.ParseDuration(s.config.httpServer.maxConnIdle); err != nil {
		s.log.Warn(ctx, "Invalid maxconnidle time for http server", klog.Fields{
			"http.server.maxconnidle": s.config.httpServer.maxConnIdle,
		})
	} else {
		maxConnIdle = t
	}
	s.log.Info(ctx, "Init http server with configuration", klog.Fields{
		"http.server.addr":          s.config.addr,
		"http.server.maxheadersize": strconv.Itoa(maxHeaderSize),
		"http.server.maxconnread":   maxConnRead.String(),
		"http.server.maxconnheader": maxConnHeader.String(),
		"http.server.maxconnwrite":  maxConnWrite.String(),
		"http.server.maxconnidle":   maxConnIdle.String(),
	})
	srv := http.Server{
		Addr:              s.config.addr,
		Handler:           s.i,
		ReadTimeout:       maxConnRead,
		ReadHeaderTimeout: maxConnHeader,
		WriteTimeout:      maxConnWrite,
		IdleTimeout:       maxConnIdle,
		MaxHeaderBytes:    maxHeaderSize,
	}
	if s.config.showBanner {
		fmt.Printf("%s\n",
			fmt.Sprintf(banner, s.config.version.Num),
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
	s.stopServices(shutdownCtx)
	return nil
}
