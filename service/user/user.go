package user

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"regexp"
	"time"
)

const (
	moduleID = "user"
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

const (
	moduleIDReqValid = moduleID + ".reqvalid"
)

var (
	emailRegex = regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]+$`)
)

const (
	lengthCap = 128
)

func (r *requestUserPost) valid() *governor.Error {
	if len(r.Username) < 3 || len(r.Username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be longer than 2 chars", 0, http.StatusBadRequest)
	}
	if len(r.Password) < 10 || len(r.Password) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "password must be longer than 9 chars", 0, http.StatusBadRequest)
	}
	if !emailRegex.MatchString(r.Email) || len(r.Email) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "email is invalid", 0, http.StatusBadRequest)
	}
	if len(r.Firstname) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "first name is too long", 0, http.StatusBadRequest)
	}
	if len(r.Lastname) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "last name is too long", 0, http.StatusBadRequest)
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

const (
	moduleIDUser = moduleID + ".user"
)

// Mount is a collection of routes for accessing and modifying user data
func (u *User) Mount(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.POST("/user", func(c echo.Context) error {
		ruser := &requestUserPost{}
		if err := c.Bind(ruser); err != nil {
			return governor.NewErrorUser(moduleIDUser, err.Error(), 0, http.StatusBadRequest)
		}
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := usermodel.NewBaseUser(ruser.Username, ruser.Password, ruser.Email, ruser.Firstname, ruser.Lastname)
		if err != nil {
			err.AddTrace(moduleIDUser)
			return err
		}
		if err := m.Insert(db); err != nil {
			err.AddTrace(moduleIDUser)
			return err
		}

		t, _ := time.Now().MarshalText()
		userid, _ := m.IDBase64()
		l.WithFields(logrus.Fields{
			"time":     string(t),
			"origin":   moduleIDUser,
			"userid":   userid,
			"username": m.Username,
		}).Info("user created")

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

// Health is a check for service health
func (u *User) Health() *governor.Error {
	return nil
}
