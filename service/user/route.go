package user

import (
	"github.com/labstack/echo/v4"
)

func (r *router) mountRoute(g *echo.Group) {
	r.mountGet(g)
	r.mountSession(g)
	r.mountCreate(g)
	r.mountEdit(g)
	r.mountEditSecure(g)
}
