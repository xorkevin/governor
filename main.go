package governor

import (
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/sirupsen/logrus"
)

type (
	// Server is an http gateway
	Server struct {
		i      *echo.Echo
		log    *logrus.Logger
		h      *health
		config Config
	}
)

// New creates a new Server
func New(config Config) (*Server, error) {
	l := newLogger(config)
	l.Info("initialized logger")
	i := echo.New()
	l.Info("initialized server instance")
	i.HTTPErrorHandler = errorHandler(i, l)
	l.Info("initialized error handling")
	i.Pre(middleware.RemoveTrailingSlash())
	if config.IsDebug() {
		i.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}, method=${method}, uri=${uri}, status=${status}, latency=${latency_human}\n",
		}))
	}
	i.Use(middleware.BodyLimit("2M"))
	i.Use(middleware.CORS())
	i.Use(middleware.Recover())
	i.Use(middleware.Gzip())
	l.Info("initialized middleware")

	s := &Server{
		i:      i,
		log:    l,
		config: config,
		h:      newHealth(),
	}
	s.MountRoute("/healthz", s.h)
	l.Info("mounted health checkpoint")

	return s, nil
}

// Start starts the server at the specified port
func (s *Server) Start() error {
	s.i.Logger.Fatal(s.i.Start(":" + s.config.Port))
	return nil
}

// Logger returns an instance to the logger
func (s *Server) Logger() *logrus.Logger {
	return s.log
}
