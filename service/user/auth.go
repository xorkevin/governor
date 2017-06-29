package user

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type (
	reqUserAuth struct {
		Username     string `json:"username"`
		Password     string `json:"password"`
		RefreshToken string `json:"refresh_token"`
	}

	reqExchangeToken struct {
		RefreshToken string `json:"refresh_token"`
	}

	resUserAuth struct {
		Valid        bool
		AccessToken  string        `json:"access_token,omitempty"`
		RefreshToken string        `json:"refresh_token,omitempty"`
		Claims       *token.Claims `json:"claims,omitempty"`
		Username     string        `json:"username,omitempty"`
		FirstName    string        `json:"first_name,omitempty"`
		LastName     string        `json:"last_name,omitempty"`
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
	return nil
}

const (
	authenticationSubject = "authentication"
	refreshSubject        = "refresh"
)

func (u *User) mountAuth(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()
	mailer := u.mailer

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
			if ok, claims := u.tokenizer.GetClaims(ruser.RefreshToken); ok {
				if s := strings.Split(claims.Id, ":"); len(s) == 2 {
					if _, err := ch.Get(s[0]).Result(); err == nil {
						if id, err := uid.FromBase64(4, 8, 4, s[0]); err == nil {
							sessionID = id.Base64()
						}
					} else {
						return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
					}
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
			refreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, sessionID+":"+sessionKey)
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			userid, err := m.IDBase64()
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			sessionIDSetKey := "usersession:" + userid

			if isMember, err := ch.HExists(sessionIDSetKey, sessionID).Result(); err == nil {
				if !isMember {
					if err := mailer.Send(m.Email, "New Login", "New login from "+c.RealIP()); err != nil {
						err.AddTrace(moduleIDAuth)
						return err
					}
				}
				if err = ch.HSet(sessionIDSetKey, sessionID, strconv.FormatInt(time.Now().Unix(), 10)+","+c.RealIP()).Err(); err != nil {
					return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
				}
			} else {
				return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
			}

			// set the session id and key into cache
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

		sessionID := ""
		sessionKey := ""
		if ok, claims := u.tokenizer.GetClaims(ruser.RefreshToken); ok {
			if s := strings.Split(claims.Id, ":"); len(s) == 2 {
				if key, err := ch.Get(s[0]).Result(); err == nil {
					sessionID = s[0]
					sessionKey = key
				} else {
					return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
				}
			}
		}

		if sessionID == "" {
			return governor.NewErrorUser(moduleIDAuth, "malformed refresh token", 0, http.StatusUnauthorized)
		}

		// check the refresh token
		validToken, claims := u.tokenizer.Validate(ruser.RefreshToken, refreshSubject, sessionID+":"+sessionKey)
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

		sessionID := ""
		sessionKey := ""
		if ok, claims := u.tokenizer.GetClaims(ruser.RefreshToken); ok {
			if s := strings.Split(claims.Id, ":"); len(s) == 2 {
				if key, err := ch.Get(s[0]).Result(); err == nil {
					sessionID = s[0]
					sessionKey = key
				} else {
					return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
				}
			}
		}

		if sessionID == "" {
			return governor.NewErrorUser(moduleIDAuth, "malformed refresh token", 0, http.StatusUnauthorized)
		}

		// check the refresh token
		validToken, claims := u.tokenizer.Validate(ruser.RefreshToken, refreshSubject, sessionID+":"+sessionKey)
		if !validToken {
			return c.JSON(http.StatusUnauthorized, &resUserAuth{
				Valid: false,
			})
		}

		// create a new key for the session
		key, err := uid.NewU(0, 16)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}
		sessionKey = key.Base64()

		// generate a new refreshToken from the refreshToken claims
		refreshToken, err := u.tokenizer.GenerateFromClaims(claims, u.accessTime, refreshSubject, sessionID+":"+sessionKey)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}

		// set the session id and key into cache
		if err := ch.Set(sessionID, sessionKey, time.Duration(u.refreshTime*b1)).Err(); err != nil {
			return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
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
