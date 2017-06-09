package user

import (
	"database/sql"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
)

type (
	requestUserAuth struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	responseUserAuth struct {
		Valid bool
	}
)

func (r *requestUserAuth) valid() *governor.Error {
	if err := hasUsername(r.Username); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func mountAuth(conf governor.Config, r *echo.Group, db *sql.DB, l *logrus.Logger) error {
	r.POST("/login", func(c echo.Context) error {
		ruser := &requestUserAuth{}
		if err := c.Bind(ruser); err != nil {
			return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		m, err := usermodel.GetByUsername(db, ruser.Username)
		if err != nil {
			return err
		}
		if m.ValidatePass(ruser.Password) {
			return c.JSON(http.StatusOK, &responseUserAuth{
				Valid: true,
			})
		}

		return c.JSON(http.StatusUnauthorized, &responseUserAuth{
			Valid: false,
		})
	})

	return nil
}
