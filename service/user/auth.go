package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/rank"
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
		Valid  bool
		Token  string        `json:"token,omitempty"`
		Claims *token.Claims `json:"claims,omitempty"`
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

const (
	moduleIDAuthMiddle    = moduleIDAuth + ".middleware"
	authenticationSubject = "authentication"
)

type (
	// Validator is a function to check the authorization of a user
	Validator func(c echo.Context, claims token.Claims) bool
)

// Authenticate builds a middleware function to validate tokens and set claims
func (u *User) Authenticate(v Validator) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := strings.Split(c.Request().Header.Get("Authorization"), " ")
			if len(h) != 2 || h[0] != "Bearer" || len(h[1]) == 0 {
				return governor.NewErrorUser(moduleIDAuthMiddle, "user is not authorized", 0, http.StatusUnauthorized)
			}
			validToken, claims := u.tokenizer.Validate(h[1], authenticationSubject, "")
			if !validToken {
				return governor.NewErrorUser(moduleIDAuthMiddle, "user is not authorized", 0, http.StatusUnauthorized)
			}
			if !v(c, *claims) {
				return governor.NewErrorUser(moduleIDAuthMiddle, "user is forbidden", 0, http.StatusForbidden)
			}
			c.Set("user", claims)
			return next(c)
		}
	}
}

// AuthenticateOwner is a middleware function to validate if a user owns the accessed resource
func (u *User) AuthenticateOwner(param string) echo.MiddlewareFunc {
	return u.Authenticate(func(c echo.Context, claims token.Claims) bool {
		return c.Param(param) == claims.Userid
	})
}

// AuthenticateAdmin is a middleware function to validate if a user is an admin
func (u *User) AuthenticateAdmin() echo.MiddlewareFunc {
	return u.Authenticate(func(c echo.Context, claims token.Claims) bool {
		return rank.FromString(claims.AuthTags).Has(rank.TagAdmin)
	})
}

// AuthenticateUser is a middleware function to validate if the request is made by a user
func (u *User) AuthenticateUser() echo.MiddlewareFunc {
	return u.Authenticate(func(c echo.Context, claims token.Claims) bool {
		return rank.FromString(claims.AuthTags).Has(rank.TagUser)
	})
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
			token, claims, err := u.tokenizer.Generate(m, u.loginTime, authenticationSubject, "")
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			return c.JSON(http.StatusOK, &responseUserAuth{
				Valid:  true,
				Token:  token,
				Claims: claims,
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
		}, u.AuthenticateUser())
	}

	return nil
}
