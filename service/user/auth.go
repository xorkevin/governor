package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/token"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
)

type (
	requestUserAuth struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	responseUserAuth struct {
		Valid     bool
		Token     string        `json:"token,omitempty"`
		Claims    *token.Claims `json:"claims,omitempty"`
		Username  string        `json:"username,omitempty"`
		FirstName string        `json:"first_name,omitempty"`
		LastName  string        `json:"last_name,omitempty"`
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

func (u *User) mountAuth(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := u.db.DB()

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
			token, claims, err := u.tokenizer.Generate(m, u.loginTime, "authentication", "")
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			return c.JSON(http.StatusOK, &responseUserAuth{
				Valid:     true,
				Token:     token,
				Claims:    claims,
				Username:  m.Username,
				FirstName: m.FirstName,
				LastName:  m.LastName,
			})
		}

		return c.JSON(http.StatusUnauthorized, &responseUserAuth{
			Valid: false,
		})
	})

	if conf.IsDebug() {
		r.GET("/decode", func(c echo.Context) error {
			return c.JSON(http.StatusOK, responseUserAuth{
				Valid:  true,
				Token:  strings.Split(c.Request().Header.Get("Authorization"), " ")[1],
				Claims: c.Get("user").(*token.Claims),
			})
		}, u.gate.User())
	}

	return nil
}
