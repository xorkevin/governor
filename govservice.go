package governor

import (
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
)

type (
	// Service is an interface for services
	Service interface {
		Mount(c Config, r *echo.Group, l *logrus.Logger) error
		Health() *Error
	}
)

// MountRoute mounts a service
func (s *Server) MountRoute(path string, r Service, m ...echo.MiddlewareFunc) error {
	s.h.addService(r)
	return r.Mount(s.config, s.i.Group(path, m...), s.log)
}
