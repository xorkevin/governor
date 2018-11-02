package governor

import (
	"github.com/labstack/echo"
)

type (
	// Service is an interface for services
	Service interface {
		Mount(c Config, l Logger, r *echo.Group) error
		Health() *Error
		Setup(c Config, l Logger, rsetup ReqSetupPost) *Error
	}
)

// MountRoute mounts a service
func (s *Server) MountRoute(path string, r Service, m ...echo.MiddlewareFunc) error {
	s.h.addService(r)
	s.s.addService(r)
	return r.Mount(s.config, s.logger, s.i.Group(s.config.BaseURL+path, m...))
}
