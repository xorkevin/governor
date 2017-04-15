package user

import (
	"database/sql"
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"regexp"
	"time"
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
	return nil
}

type (
	responseUserPost struct {
		Userid    string `json:"userid"`
		Username  string `json:"username"`
		Firstname string `json:"first_name"`
		Lastname  string `json:"last_name"`
	}
)

// Mount is a collection of routes for healthchecks
func (u *User) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
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
			l.WithFields(logrus.Fields{
				"service": "user",
				"request": "post",
				"action":  "new base user",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		if err := m.Insert(db); err != nil {
			l.WithFields(logrus.Fields{
				"service": "user",
				"request": "post",
				"action":  "insert user",
			}).Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}

		t, _ := time.Now().MarshalText()
		userid, _ := m.IDBase64()
		l.WithFields(logrus.Fields{
			"time":    string(t),
			"service": "user",
			"request": "post",
			"userid":  userid,
			"action":  "created",
		}).Info("success")

		return c.JSON(http.StatusCreated, &responseUserPost{
			Userid:    userid,
			Username:  m.Username,
			Firstname: m.FirstName,
			Lastname:  m.LastName,
		})
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
