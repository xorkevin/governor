package governor

import (
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	_ "github.com/lib/pq" // depends upon postgres
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
)

type (
	// Config is the server configuration
	Config struct {
		Version     string
		LogLevel    int
		PostgresURL string
	}
)

// NewConfig creates a new server configuration
// It requires ENV vars:
//   VERSION
//   MODE
//   POSTGRES_URL
func NewConfig() Config {
	return Config{
		Version:     os.Getenv("VERSION"),
		LogLevel:    envToLevel(os.Getenv("MODE")),
		PostgresURL: os.Getenv("POSTGRES_URL"),
	}
}

// IsDebug returns if the configuration is in debug mode
func (c *Config) IsDebug() bool {
	return c.LogLevel == levelDebug
}

type (
	// Server is an http gateway
	Server struct {
		i      *echo.Echo
		log    *logrus.Logger
		h      *health
		db     *database
		config Config
	}
)

// New creates a new Server
func New(config Config) (*Server, error) {
	l := newLogger(&config)
	l.Info("initialized logger")
	i := echo.New()
	l.Info("initialized server instance")
	i.HTTPErrorHandler = errorHandler(i, l)
	l.Info("initialized error handling")
	i.Pre(middleware.RemoveTrailingSlash())
	if config.LogLevel == levelDebug {
		i.Use(middleware.LoggerWithConfig(middleware.LoggerConfig{
			Format: "time=${time_rfc3339}, method=${method}, uri=${uri}, status=${status}, latency=${latency_human}\n",
		}))
	}
	i.Use(middleware.BodyLimit("1M"))
	i.Use(middleware.CORS())
	i.Use(middleware.Recover())
	i.Use(middleware.Gzip())
	l.Info("initialized middleware")
	db, err := newDB(&config)
	if err != nil {
		l.Error(err)
		return nil, err
	}
	l.Info("initialized database")

	s := &Server{
		i:      i,
		log:    l,
		db:     db,
		config: config,
		h:      newHealth(),
	}
	s.MountRoute("/api/healthz", s.h)
	l.Info("mounted health checkpoint")
	s.MountRoute("/api/null/database", db)
	l.Info("mounted database")

	return s, nil
}

// Start starts the server at the specified port
func (s *Server) Start(port uint) error {
	defer s.db.db.Close()

	s.i.Logger.Fatal(s.i.Start(":" + strconv.Itoa(int(port))))
	return nil
}
