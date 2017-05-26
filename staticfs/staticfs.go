package staticfs

import (
	"fmt"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/spf13/viper"
)

type (
	// Config is the server configuration
	Config struct {
		BaseDir string
		Port    string
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
func NewConfig() (Config, error) {
	v := viper.New()
	v.SetDefault("basedir", "public")
	v.SetDefault("port", "3000")

	v.SetConfigName("fsserve")
	v.AddConfigPath("./config")
	v.AddConfigPath(".")
	v.SetConfigType("yaml")

	if err := v.ReadInConfig(); err != nil {
		fmt.Println(err)
		return Config{}, err
	}

	return Config{
		BaseDir: v.GetString("basedir"),
		Port:    v.GetString("port"),
	}, nil
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
func (s *StaticFileServer) Start() error {
	s.i.Logger.Fatal(s.i.Start(":" + s.config.Port))
	return nil
}
