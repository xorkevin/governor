package user

import (
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	// User is a user management service
	User struct {
	}
)

// New creates a new User
func New() *User {
	return &User{}
}

// Mount is a collection of routes for healthchecks
func (u *User) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	ru := r.Group("/user")
	ru.GET("/:id", func(c echo.Context) error {
		return c.String(http.StatusOK, "public: "+c.Param("id"))
	})
	ru.GET("/:id/private", func(c echo.Context) error {
		return c.String(http.StatusOK, "private: "+c.Param("id"))
	})

	ra := r.Group("/auth")
	ra.GET("/login", func(c echo.Context) error {
		return c.String(http.StatusOK, "login")
	})

	return nil
}
