package user

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"regexp"
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

type (
	requestUserPost struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		Email     string `json:"email"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
	}
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

func (r *requestUserPost) valid() error {
	if len(r.Username) < 3 {
		return fmt.Errorf("username must be longer than 2 chars")
	}
	if len(r.Password) < 10 {
		return fmt.Errorf("password must be longer than 9 chars")
	}
	if !emailRegex.MatchString(r.Email) {
		return fmt.Errorf("email is invalid")
	}
	if len(r.Firstname) < 1 {
		return fmt.Errorf("first name must be provided")
	}
	if len(r.Lastname) < 1 {
		return fmt.Errorf("last name must be provided")
	}
	return nil
}

// Mount is a collection of routes for healthchecks
func (u *User) Mount(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	r.POST("/user", func(c echo.Context) error {
		ruser := &requestUserPost{}
		if err := c.Bind(ruser); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		if err := ruser.valid(); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}
		m, err := usermodel.NewBaseUser(ruser.Username, ruser.Password, ruser.Email, ruser.Firstname, ruser.Lastname)
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		return c.JSON(http.StatusCreated, m)
	})

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
