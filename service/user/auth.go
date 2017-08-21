package user

import (
	"errors"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/session"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/uid"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
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
		Valid        bool          `json:"valid"`
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

func (u *userService) setAccessCookie(c echo.Context, conf governor.Config, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     conf.BaseURL,
		Expires:  time.Now().Add(time.Duration(u.accessTime * b1)),
		HttpOnly: true,
	})
}

func (u *userService) setRefreshCookie(c echo.Context, conf governor.Config, refreshToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     conf.BaseURL + "/u/auth",
		Expires:  time.Now().AddDate(0, 6, 0),
		HttpOnly: true,
	})
}

func getAccessCookie(c echo.Context) (string, error) {
	cookie, err := c.Cookie("access_token")
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("no cookie value")
	}
	return cookie.Value, nil
}

func getRefreshCookie(c echo.Context) (string, error) {
	cookie, err := c.Cookie("refresh_token")
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("no cookie value")
	}
	return cookie.Value, nil
}

func rmAccessCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:    "access_token",
		Expires: time.Now(),
		Value:   "",
	})
}

func rmRefreshCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:    "refresh_token",
		Expires: time.Now(),
		Value:   "",
	})
}

type (
	emailNewLogin struct {
		SessionID string
		IP        string
		UserAgent string
		Time      string
	}
)

const (
	newLoginTemplate = "newlogin"
	newLoginSubject  = "newlogin_subject"
)

func (u *userService) mountAuth(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	db := u.db.DB()
	ch := u.cache.Cache()
	mailer := u.mailer

	r.POST("/login", func(c echo.Context) error {
		ruser := reqUserAuth{}
		if err := c.Bind(&ruser); err != nil {
			return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
		}
		if t, err := getRefreshCookie(c); err == nil {
			ruser.RefreshToken = t
		}
		if err := ruser.valid(); err != nil {
			return err
		}

		m, err := usermodel.GetByUsername(db, ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
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
					}
				}
			}

			var s *session.Session
			if sessionID == "" {
				// otherwise, create a new sessionID
				if s, err = session.New(m, c); err != nil {
					err.AddTrace(moduleIDAuth)
					return err
				}
			} else {
				if s, err = session.FromSessionID(sessionID, m, c); err != nil {
					err.AddTrace(moduleIDAuth)
					return err
				}
			}

			// generate an access token
			accessToken, claims, err := u.tokenizer.Generate(m, u.accessTime, authenticationSubject, "")
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}
			// generate a refresh tokens with the sessionKey
			refreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, s.SessionID+":"+s.SessionKey)
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			// store the session in cache
			if isMember, err := ch.HExists(s.UserKey(), s.SessionID).Result(); err == nil {
				sessionGob, err := s.ToGob()
				if err != nil {
					err.AddTrace(moduleIDAuth)
					return err
				}
				if !isMember {
					emdata := emailNewLogin{
						SessionID: s.SessionID,
						IP:        s.IP,
						Time:      time.Unix(s.Time, 0).String(),
						UserAgent: s.UserAgent,
					}

					em, err := u.tpl.ExecuteHTML(newLoginTemplate, emdata)
					if err != nil {
						err.AddTrace(moduleIDAuth)
						return err
					}
					subj, err := u.tpl.ExecuteHTML(newLoginSubject, emdata)
					if err != nil {
						err.AddTrace(moduleIDAuth)
						return err
					}

					if err := mailer.Send(m.Email, subj, em); err != nil {
						err.AddTrace(moduleIDAuth)
						return err
					}
				}
				if err := ch.HSet(s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
					return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
				}
			} else {
				return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
			}

			// set the session id and key into cache
			if err := ch.Set(s.SessionID, s.SessionKey, time.Duration(u.refreshTime*b1)).Err(); err != nil {
				return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
			}

			u.setAccessCookie(c, conf, accessToken)
			u.setRefreshCookie(c, conf, refreshToken)

			return c.JSON(http.StatusOK, resUserAuth{
				Valid:        true,
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				Claims:       claims,
				Username:     m.Username,
				FirstName:    m.FirstName,
				LastName:     m.LastName,
			})
		}

		return c.JSON(http.StatusUnauthorized, resUserAuth{
			Valid: false,
		})
	})

	r.POST("/exchange", func(c echo.Context) error {
		ruser := reqExchangeToken{}
		if t, err := getRefreshCookie(c); err == nil {
			ruser.RefreshToken = t
		} else if err := c.Bind(&ruser); err != nil {
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
				}
			}
		}

		if sessionID == "" {
			return governor.NewErrorUser(moduleIDAuth, "malformed refresh token", 0, http.StatusUnauthorized)
		}

		// check the refresh token
		validToken, claims := u.tokenizer.Validate(ruser.RefreshToken, refreshSubject, sessionID+":"+sessionKey)
		if !validToken {
			return c.JSON(http.StatusUnauthorized, resUserAuth{
				Valid: false,
			})
		}

		// generate a new accessToken from the refreshToken claims
		accessToken, err := u.tokenizer.GenerateFromClaims(claims, u.accessTime, authenticationSubject, "")
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}

		u.setAccessCookie(c, conf, accessToken)

		return c.JSON(http.StatusOK, resUserAuth{
			Valid:       true,
			AccessToken: accessToken,
			Claims:      claims,
		})
	})

	r.POST("/refresh", func(c echo.Context) error {
		ruser := reqExchangeToken{}
		if t, err := getRefreshCookie(c); err == nil {
			ruser.RefreshToken = t
		} else if err := c.Bind(&ruser); err != nil {
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
				}
			}
		}

		if sessionID == "" {
			return governor.NewErrorUser(moduleIDAuth, "malformed refresh token", 0, http.StatusUnauthorized)
		}

		// check the refresh token
		validToken, claims := u.tokenizer.Validate(ruser.RefreshToken, refreshSubject, sessionID+":"+sessionKey)
		if !validToken {
			return c.JSON(http.StatusUnauthorized, resUserAuth{
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

		u.setRefreshCookie(c, conf, refreshToken)

		return c.JSON(http.StatusOK, resUserAuth{
			Valid:        true,
			RefreshToken: refreshToken,
		})
	})

	r.POST("/logout", func(c echo.Context) error {
		rmAccessCookie(c)
		return c.NoContent(http.StatusNoContent)
	})

	if conf.IsDebug() {
		r.GET("/decode", func(c echo.Context) error {
			return c.JSON(http.StatusOK, resUserAuth{
				Valid:  true,
				Claims: c.Get("user").(*token.Claims),
			})
		}, gate.User(u.gate))
	}

	return nil
}
