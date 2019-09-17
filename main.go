package governor

import (
	"context"
	"fmt"
	"github.com/fatih/color"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/spf13/cobra"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"time"
	"xorkevin.dev/governor/service/state"
)

type (
	// Server is a governor server to which services may be registered
	Server struct {
		services   []serviceDef
		config     Config
		state      state.State
		logger     Logger
		i          *echo.Echo
		rootCmd    *cobra.Command
		flags      govflags
		showBanner bool
		setupRun   bool
	}
)

// New creates a new Server
func New(conf ConfigOpts, stateService state.State) *Server {
	s := &Server{
		services:   []serviceDef{},
		config:     newConfig(conf),
		state:      stateService,
		showBanner: true,
		setupRun:   false,
	}
	s.initCommand(conf)
	return s
}

// init initializes the config, creates a new logger, and initializes the
// server and its registered services
func (s *Server) init(ctx context.Context) error {
	config := s.config
	if s.flags.configFile != "" {
		config.setConfigFile(s.flags.configFile)
	}
	if err := config.init(); err != nil {
		return err
	}
	s.config = config

	s.logger = newLogger(s.config)

	i := echo.New()
	s.i = i

	l := s.logger.WithData(map[string]string{
		"phase": "init",
	})

	l.Info("init server instance", nil)

	i.HideBanner = true
	i.HTTPErrorHandler = errorHandler(i, s.logger.Subtree("errorhandler"))
	l.Info("init error handler", nil)
	i.Binder = requestBinder()
	l.Info("init request binder", nil)
	i.Pre(middleware.RemoveTrailingSlash())
	l.Info("init middleware RemoveTrailingSlash", nil)
	if len(config.RouteRewrite) > 0 {
		rewriteRules := make(map[string]string, len(config.RouteRewrite))
		for k, v := range config.RouteRewrite {
			rewriteRules["^"+k] = v
		}
		i.Pre(middleware.Rewrite(rewriteRules))
		l.Info("init route rewrite rules", config.RouteRewrite)
	}

	if config.IsDebug() {
		i.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}, method=${method}, uri=${uri}, status=${status}, latency=${latency_human}\n",
		}))
		l.Info("init request logger", nil)
	}

	i.Use(middleware.Gzip())
	l.Info("init middleware gzip", nil)

	if len(config.Origins) > 0 {
		i.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     config.Origins,
			AllowCredentials: true,
		}))
		l.Info("init middleware CORS", map[string]string{
			"origins": strings.Join(config.Origins, ", "),
		})
	}

	i.Use(middleware.BodyLimit(config.MaxReqSize))
	l.Info("init middleware body limit", map[string]string{
		"maxreqsize": config.MaxReqSize,
	})
	i.Use(middleware.Recover())
	l.Info("init middleware recover", nil)

	apiMiddlewareSkipper := func(c echo.Context) bool {
		path := c.Request().URL.EscapedPath()
		return strings.HasPrefix(path, config.BaseURL+"/") || config.BaseURL == path
	}
	if len(config.FrontendProxy) > 0 {
		targets := make([]*middleware.ProxyTarget, 0, len(config.FrontendProxy))
		for _, i := range config.FrontendProxy {
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
				"proxies": strings.Join(config.FrontendProxy, ", "),
			})
		}
	} else {
		i.Use(middleware.StaticWithConfig(middleware.StaticConfig{
			Root:    config.PublicDir,
			Index:   "index.html",
			Browse:  false,
			HTML5:   true,
			Skipper: apiMiddlewareSkipper,
		}))
		l.Info("init middleware static dir", map[string]string{
			"root":  config.PublicDir,
			"index": "index.html",
		})
	}

	i.Use(middleware.RequestID())
	l.Info("init middleware request id", nil)

	s.initSetup(i.Group(config.BaseURL + "/setupz"))
	l.Info("init setup service", nil)
	s.initHealth(i.Group(config.BaseURL + "/healthz"))
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
	if s.showBanner {
		fmt.Printf(color.BlueString(banner+"\n"), color.GreenString(s.config.Version), "build version:"+color.GreenString(s.config.VersionHash), "http server on "+color.RedString(":"+s.config.Port))
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
		s.logger.Error("init failed", map[string]string{
			"error": err.Error(),
			"phase": "setup",
		})
		return err
	}
	if err := s.setupServices(req); err != nil {
		return err
	}
	return nil
}
