package governor

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"net/url"
	"strings"
)

const (
	banner = `
  __ _  _____   _____ _ __ _ __   ___  _ __
 / _' |/ _ \ \ / / _ \ '__| '_ \ / _ \| '__|
| (_| | (_) \ V /  __/ |  | | | | (_) | |
 \__. |\___/ \_/ \___|_|  |_| |_|\___/|_|
  __/ |
 |___/  %s

 %s
 %s
`
)

type (
	// Server is an http gateway
	Server struct {
		i          *echo.Echo
		logger     Logger
		h          *health
		s          *setup
		config     Config
		showBanner bool
	}
)

// New creates a new Server
func New(config Config, l Logger) (*Server, error) {
	i := echo.New()
	i.HideBanner = true
	i.HTTPErrorHandler = errorHandler(i, l)
	l.Info("initialize error handler", nil)
	i.Binder = requestBinder()
	l.Info("initialize custom request binder", nil)
	i.Pre(middleware.RemoveTrailingSlash())
	if len(config.RouteRewrite) > 0 {
		l.Info("add route rewrite rules", config.RouteRewrite)
		rewriteRules := make(map[string]string, len(config.RouteRewrite))
		for k, v := range config.RouteRewrite {
			rewriteRules["^"+k] = v
		}
		i.Pre(middleware.Rewrite(rewriteRules))
	}

	if config.IsDebug() {
		i.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}, method=${method}, uri=${uri}, status=${status}, latency=${latency_human}\n",
		}))
	}

	i.Use(middleware.Gzip())

	if len(config.Origins) > 0 {
		i.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins:     config.Origins,
			AllowCredentials: true,
		}))
	}

	i.Use(middleware.BodyLimit(config.MaxReqSize))
	i.Use(middleware.Recover())

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
		}
	} else {
		i.Use(middleware.StaticWithConfig(middleware.StaticConfig{
			Root:    config.PublicDir,
			Index:   "index.html",
			Browse:  false,
			HTML5:   true,
			Skipper: apiMiddlewareSkipper,
		}))
	}

	i.Use(middleware.RequestID())
	l.Info("initialize middleware", nil)

	healthService := newHealth()
	if err := healthService.Mount(config, l, i.Group(config.BaseURL+"/healthz")); err != nil {
		return nil, err
	}
	setupService := newSetup()
	if err := setupService.Mount(config, l, i.Group(config.BaseURL+"/setupz")); err != nil {
		return nil, err
	}

	s := &Server{
		i:          i,
		logger:     l,
		config:     config,
		h:          healthService,
		s:          setupService,
		showBanner: true,
	}

	l.Info("initialize server instance", nil)
	return s, nil
}

// Start starts the server at the specified port
func (s *Server) Start() error {
	if s.showBanner {
		fmt.Printf(color.BlueString(banner+"\n"), color.GreenString(s.config.Version), "build version:"+color.GreenString(s.config.VersionHash), "http server on "+color.RedString(":"+s.config.Port))
	}
	s.i.Logger.Fatal(s.i.Start(":" + s.config.Port))
	return nil
}

// Must ensures that the operation must succeed
func Must(err error) {
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
}
