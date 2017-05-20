package user

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	requestUserAuth struct {
		Username string `json:"username"`
	}

	responseUserAuth struct {
	}
)

func (r *requestUserAuth) valid() *governor.Error {
	if len(r.Username) < 1 || len(r.Username) > lengthCap {
		return governor.NewErrorUser(moduleIDReqValid, "username must be provided", 0, http.StatusBadRequest)
	}
	return nil
}

func mountAuth(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.GET("/login", func(c echo.Context) error {
		return c.String(http.StatusOK, "login")
	})

	return nil
}
