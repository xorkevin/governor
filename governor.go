package governor

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
	"xorkevin.dev/governor/service/state"
)

type (
	govflags struct {
		configFile string
	}

	// Server is a governor server to which services may be registered
	Server struct {
		services []serviceDef
		config   *Config
		state    state.State
		logger   Logger
		i        *echo.Echo
		flags    govflags
		setupRun bool
	}
)

// New creates a new Server
func New(opts Opts, stateService state.State) *Server {
	return &Server{
		services: []serviceDef{},
		config:   newConfig(opts),
		state:    stateService,
		flags: govflags{
			configFile: "",
		},
		setupRun: false,
	}
}

func (s *Server) setFlags(flags govflags) {
	s.flags = flags
}

// init initializes the config, creates a new logger, and initializes the
// server and its registered services
func (s *Server) init(ctx context.Context) error {
	if s.flags.configFile != "" {
		s.config.setConfigFile(s.flags.configFile)
	}
	if err := s.config.init(); err != nil {
		return err
	}
	s.logger = newLogger(*s.config)

	l := s.logger.WithData(map[string]string{
		"phase": "init",
	})

	i := echo.New()
	s.i = i

	l.Info("init server instance", nil)

	i.HideBanner = true
	i.HTTPErrorHandler = errorHandler(i, s.logger.Subtree("errorhandler"))
	l.Info("init error handler", nil)
	i.Binder = requestBinder()
	l.Info("init request binder", nil)
	i.Pre(middleware.RemoveTrailingSlash())
	l.Info("init middleware RemoveTrailingSlash", nil)
	if len(s.config.routeRewrite) > 0 {
		rewriteRules := make(map[string]string, len(s.config.routeRewrite))
		for k, v := range s.config.routeRewrite {
			rewriteRules["^"+k] = v
		}
		i.Pre(middleware.Rewrite(rewriteRules))
		l.Info("init route rewrite rules", s.config.routeRewrite)
	}

	if s.config.IsDebug() {
		i.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}, method=${method}, uri=${uri}, status=${status}, latency=${latency_human}\n",
		}))
		l.Info("init request logger", nil)
	}

	i.Use(middleware.Gzip())
	l.Info("init middleware gzip", nil)

	if len(s.config.origins) > 0 {
		i.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     s.config.origins,
			AllowCredentials: true,
		}))
		l.Info("init middleware CORS", map[string]string{
			"origins": strings.Join(s.config.origins, ", "),
		})
	}

	i.Use(middleware.BodyLimit(s.config.maxReqSize))
	l.Info("init middleware body limit", map[string]string{
		"maxreqsize": s.config.maxReqSize,
	})
	i.Use(middleware.Recover())
	l.Info("init middleware recover", nil)

	apiMiddlewareSkipper := func(c echo.Context) bool {
		path := c.Request().URL.EscapedPath()
		return strings.HasPrefix(path, s.config.BaseURL+"/") || s.config.BaseURL == path
	}
	if len(s.config.frontendProxy) > 0 {
		targets := make([]*middleware.ProxyTarget, 0, len(s.config.frontendProxy))
		for _, i := range s.config.frontendProxy {
			if u, err := url.Parse(i); err == nil {
				targets = append(targets, &middleware.ProxyTarget{
					URL: u,
				})
			} else {
				l.Warn("fail add frontend proxy", map[string]string{
					"proxy": i,
					"error": err.Error(),
				})
			}
		}
		if len(targets) > 0 {
			i.Use(middleware.ProxyWithConfig(middleware.ProxyConfig{
				Balancer: middleware.NewRoundRobinBalancer(targets),
				Skipper:  apiMiddlewareSkipper,
			}))
			l.Info("init middleware frontend proxy", map[string]string{
				"proxies": strings.Join(s.config.frontendProxy, ", "),
			})
		}
	} else {
		i.Use(middleware.StaticWithConfig(middleware.StaticConfig{
			Root:    s.config.publicDir,
			Index:   "index.html",
			Browse:  false,
			HTML5:   true,
			Skipper: apiMiddlewareSkipper,
		}))
		l.Info("init middleware static dir", map[string]string{
			"root":  s.config.publicDir,
			"index": "index.html",
		})
	}

	i.Use(middleware.RequestID())
	l.Info("init middleware request id", nil)

	s.initSetup(i.Group(s.config.BaseURL + "/setupz"))
	l.Info("init setup service", nil)
	s.initHealth(i.Group(s.config.BaseURL + "/healthz"))
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
	go func() {
		if err := s.i.Start(":" + s.config.Port); err != nil {
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
	if err := s.i.Shutdown(shutdownCtx); err != nil {
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
