package governor

import (
	"compress/gzip"
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"xorkevin.dev/governor/service/state"
	"xorkevin.dev/governor/util/bytefmt"
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
		logger        Logger
		i             chi.Router
		flags         Flags
		firstSetupRun bool
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
	s.logger = newLogger(*s.config)

	l := s.logger.WithData(map[string]string{
		"phase": "init",
	})

	i := chi.NewRouter()
	s.i = i

	l.Info("init server instance", nil)

	i.Use(stripSlashesMiddleware)
	l.Info("init strip slashes middleware", nil)

	i.Use(realIPMiddleware(nil))
	l.Info("init real ip middleware", nil)

	i.Use(s.reqLoggerMiddleware)
	l.Info("init request logger", nil)

	if len(s.config.rewrite) > 0 {
		k := make([]string, 0, len(s.config.rewrite))
		for _, i := range s.config.rewrite {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.String())
		}
		i.Use(routeRewriteMiddleware(s.config.rewrite))
		l.Info("init route rewriter middleware", map[string]string{
			"rules": strings.Join(k, "; "),
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
		l.Info("init middleware allow all cors", map[string]string{
			"paths": strings.Join(k, "; "),
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
		l.Info("init middleware CORS", map[string]string{
			"origins": strings.Join(s.config.origins, ", "),
		})
	}

	if limit, err := bytefmt.ToBytes(s.config.maxReqSize); err != nil {
		l.Warn("invalid maxreqsize format for middlware body limit", map[string]string{
			"maxreqsize": s.config.maxReqSize,
		})
	} else {
		i.Use(s.bodyLimitMiddleware(limit))
		l.Info("init middleware body limit", map[string]string{
			"maxreqsize": s.config.maxReqSize,
		})
	}

	i.Use(middleware.Compress(gzip.DefaultCompression))
	l.Info("init middleware gzip", nil)

	i.Use(middleware.Recoverer)
	l.Info("init middleware Recoverer", nil)

	s.initSetup(s.router(s.config.BaseURL + "/setupz"))
	l.Info("init setup service", nil)
	s.initHealth(s.router(s.config.BaseURL + "/healthz"))
	l.Info("init health service", nil)

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
	if err := s.init(ctx); err != nil {
		if s.logger != nil {
			s.logger.Error("init failed", map[string]string{
				"error": err.Error(),
				"phase": "init",
			})
		} else {
			fmt.Println(err)
		}
		return err
	}
	l := s.logger.WithData(map[string]string{
		"phase": "start",
	})
	if err := s.startServices(ctx); err != nil {
		return err
	}

	maxHeaderSize := defaultMaxHeaderSize
	if limit, err := bytefmt.ToBytes(s.config.maxHeaderSize); err != nil {
		l.Warn("Invalid maxheadersize format for http server", map[string]string{
			"maxreqsize": s.config.maxReqSize,
		})
	} else {
		maxHeaderSize = int(limit)
	}
	maxConnRead := seconds5
	if t, err := time.ParseDuration(s.config.maxConnRead); err != nil {
		l.Warn("Invalid maxconnread time for http server", map[string]string{
			"maxconnread": s.config.maxConnRead,
		})
	} else {
		maxConnRead = t
	}
	maxConnHeader := seconds2
	if t, err := time.ParseDuration(s.config.maxConnHeader); err != nil {
		l.Warn("Invalid maxconnheader time for http server", map[string]string{
			"maxconnheader": s.config.maxConnHeader,
		})
	} else {
		maxConnHeader = t
	}
	maxConnWrite := seconds5
	if t, err := time.ParseDuration(s.config.maxConnWrite); err != nil {
		l.Warn("Invalid maxconnwrite time for http server", map[string]string{
			"maxconnwrite": s.config.maxConnWrite,
		})
	} else {
		maxConnWrite = t
	}
	maxConnIdle := seconds5
	if t, err := time.ParseDuration(s.config.maxConnIdle); err != nil {
		l.Warn("Invalid maxconnidle time for http server", map[string]string{
			"maxconnidle": s.config.maxConnIdle,
		})
	} else {
		maxConnIdle = t
	}
	l.Info("Init http server with configuration", map[string]string{
		"maxheadersize": strconv.Itoa(maxHeaderSize),
		"maxconnread":   maxConnRead.String(),
		"maxconnheader": maxConnHeader.String(),
		"maxconnwrite":  maxConnWrite.String(),
		"maxconnidle":   maxConnIdle.String(),
	})
	srv := http.Server{
		Addr:              ":" + s.config.Port,
		Handler:           s.i,
		ReadTimeout:       maxConnRead,
		ReadHeaderTimeout: maxConnHeader,
		WriteTimeout:      maxConnWrite,
		IdleTimeout:       maxConnIdle,
		MaxHeaderBytes:    maxHeaderSize,
	}
	if s.config.showBanner {
		fmt.Printf("%s\n%s: %s\nhttp server listening on %s\n",
			fmt.Sprintf(banner, s.config.version.Num),
			s.config.appname,
			s.config.version.String(),
			":"+s.config.Port)
	}
	go func() {
		defer cancel()
		if err := srv.ListenAndServe(); err != nil {
			l.Info("Shutting down server", map[string]string{
				"error": err.Error(),
			})
		}
	}()
	s.waitForInterrupt(ctx)
	l.Info("Shutdown process begin", nil)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 16*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		l.Error("Shutdown server error", map[string]string{
			"error": err.Error(),
		})
	}
	s.stopServices(shutdownCtx)
	return nil
}
