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
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"xorkevin.dev/governor/service/state"
	"xorkevin.dev/governor/util/bytefmt"
	"xorkevin.dev/kerrors"
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
		log           Logger
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
	s.log = NewLogger(LogOptMinLevelStr(s.config.logLevel))

	i := chi.NewRouter()
	s.i = i

	LogInfo(s.log, ctx, "Init server instance", nil)

	i.Use(stripSlashesMiddleware)
	LogInfo(s.log, ctx, "Init strip slashes middleware", nil)

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
		LogInfo(s.log, ctx, "Init real ip middleware", LogFields{
			"realip.proxies": strings.Join(s.config.proxies, ","),
		})
	} else {
		i.Use(realIPMiddleware(nil))
		LogInfo(s.log, ctx, "Init real ip middleware", nil)
	}

	i.Use(s.reqLoggerMiddleware)
	LogInfo(s.log, ctx, "Init request logger", nil)

	if len(s.config.rewrite) > 0 {
		k := make([]string, 0, len(s.config.rewrite))
		for _, i := range s.config.rewrite {
			if err := i.init(); err != nil {
				return err
			}
			k = append(k, i.String())
		}
		i.Use(routeRewriteMiddleware(s.config.rewrite))
		LogInfo(s.log, ctx, "Init route rewriter middleware", LogFields{
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
		LogInfo(s.log, ctx, "Init middleware allow all cors", LogFields{
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
		LogInfo(s.log, ctx, "Init middleware CORS", LogFields{
			"cors.origins": strings.Join(s.config.origins, ", "),
		})
	}

	if limit, err := bytefmt.ToBytes(s.config.maxReqSize); err != nil {
		LogWarn(s.log, ctx, "Invalid maxreqsize format for middlware body limit", LogFields{
			"bodylimit.maxreqsize": s.config.maxReqSize,
		})
	} else {
		i.Use(s.bodyLimitMiddleware(limit))
		LogInfo(s.log, ctx, "Init middleware body limit", LogFields{
			"bodylimit.maxreqsize": s.config.maxReqSize,
		})
	}

	i.Use(compressorMiddleware())
	LogInfo(s.log, ctx, "Init middleware gzip", nil)

	i.Use(middleware.Recoverer)
	LogInfo(s.log, ctx, "Init middleware recoverer", nil)

	s.initSetup(s.router(s.config.BaseURL + "/setupz"))
	LogInfo(s.log, ctx, "Init setup routes", nil)
	s.initHealth(s.router(s.config.BaseURL + "/healthz"))
	LogInfo(s.log, ctx, "Init health routes", nil)

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
	ctx = LogWithFields(ctx, LogFields{
		"gov.service.phase": "init",
	})
	if err := s.init(ctx); err != nil {
		if s.log != nil {
			LogErr(s.log, ctx, kerrors.WithMsg(err, "Init failed"), nil)
		} else {
			fmt.Println(err)
		}
		return err
	}
	ctx = LogWithFields(ctx, LogFields{
		"gov.service.phase": "start",
	})
	if err := s.startServices(ctx); err != nil {
		return err
	}

	maxHeaderSize := defaultMaxHeaderSize
	if limit, err := bytefmt.ToBytes(s.config.maxHeaderSize); err != nil {
		LogWarn(s.log, ctx, "Invalid maxheadersize format for http server", LogFields{
			"http.server.maxheadersize": s.config.maxReqSize,
		})
	} else {
		maxHeaderSize = int(limit)
	}
	maxConnRead := seconds5
	if t, err := time.ParseDuration(s.config.maxConnRead); err != nil {
		LogWarn(s.log, ctx, "Invalid maxconnread time for http server", LogFields{
			"http.server.maxconnread": s.config.maxConnRead,
		})
	} else {
		maxConnRead = t
	}
	maxConnHeader := seconds2
	if t, err := time.ParseDuration(s.config.maxConnHeader); err != nil {
		LogWarn(s.log, ctx, "Invalid maxconnheader time for http server", LogFields{
			"http.server.maxconnheader": s.config.maxConnHeader,
		})
	} else {
		maxConnHeader = t
	}
	maxConnWrite := seconds5
	if t, err := time.ParseDuration(s.config.maxConnWrite); err != nil {
		LogWarn(s.log, ctx, "Invalid maxconnwrite time for http server", LogFields{
			"http.server.maxconnwrite": s.config.maxConnWrite,
		})
	} else {
		maxConnWrite = t
	}
	maxConnIdle := seconds5
	if t, err := time.ParseDuration(s.config.maxConnIdle); err != nil {
		LogWarn(s.log, ctx, "Invalid maxconnidle time for http server", LogFields{
			"http.server.maxconnidle": s.config.maxConnIdle,
		})
	} else {
		maxConnIdle = t
	}
	LogInfo(s.log, ctx, "Init http server with configuration", LogFields{
		"http.server.maxheadersize": strconv.Itoa(maxHeaderSize),
		"http.server.maxconnread":   maxConnRead.String(),
		"http.server.maxconnheader": maxConnHeader.String(),
		"http.server.maxconnwrite":  maxConnWrite.String(),
		"http.server.maxconnidle":   maxConnIdle.String(),
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
		fmt.Printf("%s\n%s: %s\nhostname: %s\nhttp server listening on :%s\nsecrets loaded from %s\n",
			fmt.Sprintf(banner, s.config.version.Num),
			s.config.appname,
			s.config.version.String(),
			s.config.Hostname,
			s.config.Port,
			s.config.vault.Info(),
		)
	}
	go func() {
		defer cancel()
		if err := srv.ListenAndServe(); err != nil {
			LogErr(s.log, ctx, kerrors.WithMsg(err, "Shutting down server"), nil)
		}
	}()
	s.waitForInterrupt(ctx)
	ctx = LogWithFields(ctx, LogFields{
		"gov.service.phase": "stop",
	})
	LogInfo(s.log, ctx, "Shutdown process begin", nil)
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(LogExtendCtx(context.Background(), ctx, nil), 16*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		LogErr(s.log, shutdownCtx, kerrors.WithMsg(err, "Shutdown server error"), nil)
	}
	s.stopServices(shutdownCtx)
	return nil
}
