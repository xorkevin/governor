package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

type (
	reqUserAuth struct {
		Username  string `json:"username"`
		Password  string `json:"password"`
		SessionID string `json:"session_id"`
	}

	reqExchangeToken struct {
		RefreshToken string `json:"refresh_token"`
		SessionID    string `json:"session_id"`
	}

	resUserAuth struct {
		Valid        bool
		AccessToken  string        `json:"access_token,omitempty"`
		RefreshToken string        `json:"refresh_token,omitempty"`
		Claims       *token.Claims `json:"claims,omitempty"`
		Username     string        `json:"username,omitempty"`
		FirstName    string        `json:"first_name,omitempty"`
		LastName     string        `json:"last_name,omitempty"`
		SessionID    string        `json:"session_id,omitempty"`
	}
)

func (r *reqUserAuth) valid() *governor.Error {
	if err := hasUsername(r.Username); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r *reqExchangeToken) valid() *governor.Error {
	if err := hasToken(r.RefreshToken); err != nil {
		return err
	}
	if err := hasToken(r.SessionID); err != nil {
		return err
	}
	return nil
}

const (
	authenticationSubject = "authentication"
	refreshSubject        = "refresh"
)

func (u *User) mountAuth(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()

	r.POST("/login", func(c echo.Context) error {
		ruser := &reqUserAuth{}
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
			sessionID := ""
			// if session_id is provided, is in cache, and is valid, set it as the sessionID
			if len(ruser.SessionID) > 0 {
				if _, err := ch.Get(ruser.SessionID).Result(); err == nil {
					if id, err := uid.FromBase64(4, 8, 4, ruser.SessionID); err == nil {
						sessionID = id.Base64()
					}
				} else {
					return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
				}
			}
			// otherwise, create a new sessionID
			if sessionID == "" {
				id, err := uid.New(4, 8, 4, m.Userid)
				if err != nil {
					err.AddTrace(moduleIDAuth)
					return err
				}
				sessionID = id.Base64()
			}

			// create a key for the session
			key, err := uid.NewU(0, 16)
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}
			sessionKey := key.Base64()

			// generate an access token
			accessToken, claims, err := u.tokenizer.Generate(m, u.accessTime, authenticationSubject, "")
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}
			// generate a refresh tokens with the sessionKey
			refreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, sessionKey)
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			// set the session id and key into redis
			if err := ch.Set(sessionID, sessionKey, time.Duration(u.refreshTime*b1)).Err(); err != nil {
				return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
			}

			return c.JSON(http.StatusOK, &resUserAuth{
				Valid:        true,
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				Claims:       claims,
				Username:     m.Username,
				FirstName:    m.FirstName,
				LastName:     m.LastName,
				SessionID:    sessionID,
			})
		}

		return c.JSON(http.StatusUnauthorized, &resUserAuth{
			Valid: false,
		})
	})

	r.POST("/exchange", func(c echo.Context) error {
		ruser := &reqExchangeToken{}
		if err := c.Bind(ruser); err != nil {
			return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		sessionKey := ""

		if key, err := ch.Get(ruser.SessionID).Result(); err == nil {
			sessionKey = key
		} else {
			return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
		}

		// check the refresh token
		validToken, claims := u.tokenizer.Validate(ruser.RefreshToken, refreshSubject, sessionKey)
		if !validToken {
			return c.JSON(http.StatusUnauthorized, &resUserAuth{
				Valid: false,
			})
		}

		// generate a new accessToken from the refreshToken claims
		accessToken, err := u.tokenizer.GenerateFromClaims(claims, u.accessTime, authenticationSubject, "")
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}

		return c.JSON(http.StatusOK, &resUserAuth{
			Valid:       true,
			AccessToken: accessToken,
		})
	})

	r.POST("/refresh", func(c echo.Context) error {
		ruser := &reqExchangeToken{}
		if err := c.Bind(ruser); err != nil {
			return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		// check the refresh token
		validToken, claims := u.tokenizer.Validate(ruser.RefreshToken, refreshSubject, "")
		if !validToken {
			return c.JSON(http.StatusUnauthorized, &resUserAuth{
				Valid: false,
			})
		}

		// generate a new refreshToken from the refreshToken claims
		refreshToken, err := u.tokenizer.GenerateFromClaims(claims, u.accessTime, refreshSubject, "")
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}

		return c.JSON(http.StatusOK, &resUserAuth{
			Valid:        true,
			RefreshToken: refreshToken,
		})
	})

	if conf.IsDebug() {
		r.GET("/decode", func(c echo.Context) error {
			return c.JSON(http.StatusOK, resUserAuth{
				Valid:  true,
				Claims: c.Get("user").(*token.Claims),
			})
		}, u.gate.User())
	}

	return nil
}
