package user

import (
	"github.com/labstack/echo"
)

func (r *router) mountRoute(debugMode bool, g *echo.Group) error {
	if err := r.mountGet(debugMode, g); err != nil {
		return err
	}
	if err := r.mountSession(debugMode, g); err != nil {
		return err
	}
	if err := r.mountCreate(debugMode, g); err != nil {
		return err
	}
	if err := r.mountEdit(debugMode, g); err != nil {
		return err
	}
	if err := r.mountEditSecure(debugMode, g); err != nil {
		return err
	}
	return nil
}
