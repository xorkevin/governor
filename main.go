package governor

import (
	"fmt"
	"github.com/fatih/color"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/sirupsen/logrus"
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
		log        *logrus.Logger
		h          *health
		s          *setup
		config     Config
		showBanner bool
	}
)

// New creates a new Server
func New(config Config) (*Server, error) {
	l := newLogger(config)
	l.Info("initialized logger")
	i := echo.New()
	l.Info("initialized server instance")
	i.HideBanner = true
	i.HTTPErrorHandler = errorHandler(i, l)
	l.Info("initialized error handling")
	i.Binder = requestBinder()
	l.Info("added custom request binder")
	i.Pre(middleware.RemoveTrailingSlash())

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
				l.Warnf("could not add frontend proxy %s: %s", i, err.Error())
			}
		}
		if len(targets) > 0 {
			i.Use(middleware.ProxyWithConfig(middleware.ProxyConfig{
				Balancer: &middleware.RoundRobinBalancer{
					Targets: targets,
				},
				Skipper: apiMiddlewareSkipper,
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
	l.Info("initialized middleware")

	healthService := newHealth()
	if err := healthService.Mount(config, i.Group(config.BaseURL+"/healthz"), l); err != nil {
		return nil, err
	}
	setupService := newSetup()
	if err := setupService.Mount(config, i.Group(config.BaseURL+"/setupz"), l); err != nil {
		return nil, err
	}

	s := &Server{
		i:          i,
		log:        l,
		config:     config,
		h:          healthService,
		s:          setupService,
		showBanner: true,
	}

	l.Info("server instance created")
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

// Logger returns an instance to the logger
func (s *Server) Logger() *logrus.Logger {
	return s.log
}

// Must ensures that the operation must succeed
func Must(err error) {
	if err != nil {
		fmt.Println(err)
		panic(err)
	}
}
