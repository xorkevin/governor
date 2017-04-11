package staticfs

import (
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"os"
	"strconv"
)

type (
	// Config is the server configuration
	Config struct {
		BaseDir string
	}

	// StaticFileServer is a server that serves static files
	StaticFileServer struct {
		i      *echo.Echo
		config Config
	}
)

// NewConfig creates a new server configuration
// It requires ENV vars:
//   BASEDIR
func NewConfig() Config {
	return Config{
		BaseDir: os.Getenv("BASEDIR"),
	}
}

// New creates a new StaticFileServer
func New(config Config) *StaticFileServer {
	// http server instance
	i := echo.New()

	// middleware
	i.Pre(middleware.RemoveTrailingSlash())
	i.Use(middleware.BodyLimit("2M"))
	i.Use(middleware.CORS())
	i.Use(middleware.Recover())
	i.Use(middleware.Gzip())
	i.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:  config.BaseDir,
		HTML5: true,
	}))
	return &StaticFileServer{
		i:      i,
		config: config,
	}
}

// Start starts the static file server
func (s *StaticFileServer) Start(port int) error {
	s.i.Logger.Fatal(s.i.Start(":" + strconv.Itoa(int(port))))
	return nil
}
