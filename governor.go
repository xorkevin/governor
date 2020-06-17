package governor

import (
	"compress/gzip"
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/cors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"time"
	"xorkevin.dev/governor/service/state"
	"xorkevin.dev/governor/util/bytefmt"
)

type (
	Flags struct {
		ConfigFile string
	}

	// Server is a governor server to which services may be registered
	Server struct {
		services []serviceDef
		config   *Config
		state    state.State
		logger   Logger
		i        chi.Router
		flags    Flags
		setupRun bool
	}
)

// New creates a new Server
func New(opts Opts, stateService state.State) *Server {
	return &Server{
		services: []serviceDef{},
		config:   newConfig(opts),
		state:    stateService,
		flags: Flags{
			ConfigFile: "",
		},
		setupRun: false,
	}
}

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

	i.Use(middleware.StripSlashes)
	l.Info("init middleware StripSlashes", nil)

	if s.config.IsDebug() {
		i.Use(s.reqLoggerMiddleware)
		l.Info("init request logger", nil)
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

	if len(s.config.origins) > 0 {
		i.Use(cors.Handler(cors.Options{
			AllowedOrigins:   s.config.origins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			AllowedHeaders:   []string{"*"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
		l.Info("init middleware CORS", map[string]string{
			"origins": strings.Join(s.config.origins, ", "),
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
	if s.config.showBanner {
		fmt.Printf("%s\n%s: %s\nhttp server listening on %s\n",
			fmt.Sprintf(color.BlueString(banner), s.config.version.Num),
			s.config.appname,
			color.GreenString(s.config.version.String()),
			color.RedString(":"+s.config.Port))
	}
	srv := http.Server{
		Addr:    ":" + s.config.Port,
		Handler: s.i,
	}
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			l.Info("shutting down server", map[string]string{
				"error": err.Error(),
			})
		}
	}()
	sigShutdown := make(chan os.Signal)
	signal.Notify(sigShutdown, os.Interrupt)
	<-sigShutdown
	l.Info("shutdown process begin", nil)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 16*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		l.Error("shutdown server error", map[string]string{
			"error": err.Error(),
		})
	}
	s.stopServices(shutdownCtx)
	return nil
}

// Setup runs the setup procedures for all services
func (s *Server) Setup(req ReqSetup) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := s.init(ctx); err != nil {
		if s.logger != nil {
			s.logger.Error("init failed", map[string]string{
				"error": err.Error(),
				"phase": "setup",
			})
		} else {
			fmt.Println(err)
		}
		return err
	}
	if err := s.setupServices(req); err != nil {
		return err
	}
	return nil
}
