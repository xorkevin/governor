package governor

import (
	"context"
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/cors"
	"xorkevin.dev/governor/service/state"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	defaultMaxHeaderSize = 1 << 20 // 1MB

	seconds5 = 5 * time.Second
	seconds2 = 2 * time.Second
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
		services      []serviceDef
		inj           Injector
		config        *Config
		state         state.State
		log           *klog.LevelLogger
		i             chi.Router
		flags         Flags
		firstSetupRun bool
		mu            *sync.RWMutex
	}
)

// New creates a new Server
func New(opts Opts, stateService state.State) *Server {
	return &Server{
		services: []serviceDef{},
		inj:      newInjector(context.Background()),
		config:   newConfig(opts),
		state:    stateService,
		flags: Flags{
			ConfigFile: "",
		},
		firstSetupRun: false,
		mu:            &sync.RWMutex{},
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

	if len(s.config.proxies) > 0 {
		proxies := make([]net.IPNet, 0, len(s.config.proxies))
		for _, i := range s.config.proxies {
			_, k, err := net.ParseCIDR(i)
			if err != nil {
				return err
			}
			proxies = append(proxies, *k)
		}
		i.Use(realIPMiddleware(proxies))
		s.log.Info(ctx, "Init real ip middleware", klog.Fields{
			"realip.proxies": strings.Join(s.config.proxies, ","),
		})
	} else {
		i.Use(realIPMiddleware(nil))
		s.log.Info(ctx, "Init real ip middleware", nil)
	}

	i.Use(s.reqLoggerMiddleware)
	s.log.Info(ctx, "Init request logger", nil)

	if len(s.config.rewrite) > 0 {
		k := make([]string, 0, len(s.config.rewrite))
		for _, i := range s.config.rewrite {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.String())
		}
		i.Use(routeRewriteMiddleware(s.config.rewrite))
		s.log.Info(ctx, "Init route rewriter middleware", klog.Fields{
			"routerrewrite.rules": strings.Join(k, "; "),
		})
	}

	if len(s.config.allowpaths) > 0 {
		k := make([]string, 0, len(s.config.allowpaths))
		for _, i := range s.config.allowpaths {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.pattern)
		}
		i.Use(corsPathsAllowAllMiddleware(s.config.allowpaths))
		s.log.Info(ctx, "Init middleware allow all cors", klog.Fields{
			"cors.paths": strings.Join(k, "; "),
		})
	}
	if len(s.config.origins) > 0 {
		i.Use(cors.Handler(cors.Options{
			AllowedOrigins: s.config.origins,
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
			"cors.origins": strings.Join(s.config.origins, ", "),
		})
	}

	if limit, err := bytefmt.ToBytes(s.config.maxReqSize); err != nil {
		s.log.Warn(ctx, "Invalid maxreqsize format for middlware body limit", klog.Fields{
			"bodylimit.maxreqsize": s.config.maxReqSize,
		})
	} else {
		i.Use(s.bodyLimitMiddleware(limit))
		s.log.Info(ctx, "Init middleware body limit", klog.Fields{
			"bodylimit.maxreqsize": s.config.maxReqSize,
		})
	}

	i.Use(compressorMiddleware)
	s.log.Info(ctx, "Init middleware gzip", nil)

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
	if limit, err := bytefmt.ToBytes(s.config.maxHeaderSize); err != nil {
		s.log.Warn(ctx, "Invalid maxheadersize format for http server", klog.Fields{
			"http.server.maxheadersize": s.config.maxReqSize,
		})
	} else {
		maxHeaderSize = int(limit)
	}
	maxConnRead := seconds5
	if t, err := time.ParseDuration(s.config.maxConnRead); err != nil {
		s.log.Warn(ctx, "Invalid maxconnread time for http server", klog.Fields{
			"http.server.maxconnread": s.config.maxConnRead,
		})
	} else {
		maxConnRead = t
	}
	maxConnHeader := seconds2
	if t, err := time.ParseDuration(s.config.maxConnHeader); err != nil {
		s.log.Warn(ctx, "Invalid maxconnheader time for http server", klog.Fields{
			"http.server.maxconnheader": s.config.maxConnHeader,
		})
	} else {
		maxConnHeader = t
	}
	maxConnWrite := seconds5
	if t, err := time.ParseDuration(s.config.maxConnWrite); err != nil {
		s.log.Warn(ctx, "Invalid maxconnwrite time for http server", klog.Fields{
			"http.server.maxconnwrite": s.config.maxConnWrite,
		})
	} else {
		maxConnWrite = t
	}
	maxConnIdle := seconds5
	if t, err := time.ParseDuration(s.config.maxConnIdle); err != nil {
		s.log.Warn(ctx, "Invalid maxconnidle time for http server", klog.Fields{
			"http.server.maxconnidle": s.config.maxConnIdle,
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
