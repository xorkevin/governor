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
		SessionToken string `json:"session_token"`
	}

	reqExchangeToken struct {
		RefreshToken string `json:"refresh_token"`
	}

	resUserAuth struct {
		Valid        bool          `json:"valid"`
		AccessToken  string        `json:"access_token,omitempty"`
		RefreshToken string        `json:"refresh_token,omitempty"`
		SessionToken string        `json:"session_token,omitempty"`
		Claims       *token.Claims `json:"claims,omitempty"`
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

func (r *reqUserAuth) validEmail() *governor.Error {
	if err := validEmail(r.Username); err != nil {
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
	sessionSubject        = "session"
)

func (u *userService) setAccessCookie(c echo.Context, conf governor.Config, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     conf.BaseURL,
		MaxAge:   int(u.accessTime),
		HttpOnly: false,
	})
}

const (
	month6 = 43200 * 365
)

func (u *userService) setRefreshCookie(c echo.Context, conf governor.Config, refreshToken string, authTags string) {
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     conf.BaseURL + "/u/auth",
		MaxAge:   month6,
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "refresh_valid",
		Value:    "valid",
		Path:     "/",
		MaxAge:   month6,
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "auth_tags",
		Value:    authTags,
		Path:     "/",
		MaxAge:   month6,
		HttpOnly: false,
	})
}

func (u *userService) setSessionCookie(c echo.Context, conf governor.Config, sessionToken, userid string) {
	ub64 := strings.TrimRight(userid, "=")
	c.SetCookie(&http.Cookie{
		Name:     "session_token_" + ub64,
		Value:    sessionToken,
		Path:     conf.BaseURL + "/u/auth/login",
		MaxAge:   month6,
		HttpOnly: false,
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

func getSessionCookie(c echo.Context, userid string) (string, error) {
	ub64 := strings.TrimRight(userid, "=")
	if ub64 == "" {
		return "", errors.New("no cookie value")
	}
	cookie, err := c.Cookie("session_token_" + ub64)
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("no cookie value")
	}
	return cookie.Value, nil
}

func rmAccessCookie(c echo.Context, conf governor.Config) {
	c.SetCookie(&http.Cookie{
		Name:   "access_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   conf.BaseURL,
	})
}

func rmRefreshCookie(c echo.Context, conf governor.Config) {
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   conf.BaseURL + "/u/auth",
	})
	c.SetCookie(&http.Cookie{
		Name:   "refresh_valid",
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
	c.SetCookie(&http.Cookie{
		Name:   "auth_tags",
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
}

func rmSessionCookie(c echo.Context, conf governor.Config, userid string) {
	ub64 := strings.TrimRight(userid, "=")
	c.SetCookie(&http.Cookie{
		Name:   "session_token_" + ub64,
		Value:  "invalid",
		MaxAge: -1,
		Path:   conf.BaseURL + "/u/auth/login",
	})
}

type (
	emailNewLogin struct {
		FirstName string
		Username  string
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
		isEmail := false
		if err := ruser.validEmail(); err == nil {
			isEmail = true
		}
		var m *usermodel.Model
		if isEmail {
			mu, err := usermodel.GetByEmail(db, ruser.Username)
			if err != nil {
				if err.Code() == 2 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDAuth)
				return err
			}
			m = mu
		} else {
			if err := ruser.valid(); err != nil {
				return err
			}
			mu, err := usermodel.GetByUsername(db, ruser.Username)
			if err != nil {
				if err.Code() == 2 {
					err.SetErrorUser()
				}
				err.AddTrace(moduleIDAuth)
				return err
			}
			m = mu
		}
		userid, err := m.IDBase64()
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}
		if t, err := getSessionCookie(c, userid); err == nil {
			ruser.SessionToken = t
		}

		if m.ValidatePass(ruser.Password) {
			sessionID := ""
			isMember := false
			// if claims userid matches model, session_id is provided,
			// is in list of user sessions, set it as the sessionID
			// the session can be expired by time
			if ok, claims := u.tokenizer.GetClaims(ruser.SessionToken, sessionSubject); ok {
				if userid == claims.Userid {
					usersession := session.Session{
						Userid: claims.Userid,
					}
					userkey := usersession.UserKey()
					if isM, err := ch.HExists(userkey, claims.Id).Result(); err == nil && isM {
						sessionID = claims.Id
						isMember = isM
					} else {
						if err != nil {
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
				if s, err = session.FromSessionID(sessionID, userid, c); err != nil {
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
			// generate a refresh token with the sessionKey
			refreshToken, _, err := u.tokenizer.Generate(m, u.refreshTime, refreshSubject, s.SessionID+":"+s.SessionKey)
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}
			// generate a session token
			sessionToken, _, err := u.tokenizer.Generate(m, u.refreshTime, sessionSubject, s.SessionID)
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}

			// store the session in cache
			sessionGob, err := s.ToGob()
			if err != nil {
				err.AddTrace(moduleIDAuth)
				return err
			}
			if u.newLoginEmail && !isMember {
				emdata := emailNewLogin{
					FirstName: m.FirstName,
					Username:  m.Username,
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

			// add to list of user sessions
			if err := ch.HSet(s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
				return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
			}

			// set the session id and key into cache
			if err := ch.Set(s.SessionID, s.SessionKey, time.Duration(u.refreshTime*b1)).Err(); err != nil {
				return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
			}

			u.setAccessCookie(c, conf, accessToken)
			u.setRefreshCookie(c, conf, refreshToken, claims.AuthTags)
			u.setSessionCookie(c, conf, sessionToken, userid)

			return c.JSON(http.StatusOK, resUserAuth{
				Valid:        true,
				AccessToken:  accessToken,
				RefreshToken: refreshToken,
				SessionToken: sessionToken,
				Claims:       claims,
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
		userid := ""
		// if session_id is provided, is in cache, and is valid, set it as the sessionID
		// the session cannot be expired
		if ok, claims := u.tokenizer.GetClaims(ruser.RefreshToken, refreshSubject); ok {
			if s := strings.Split(claims.Id, ":"); len(s) == 2 {
				if key, err := ch.Get(s[0]).Result(); err == nil {
					sessionID = s[0]
					sessionKey = key
					userid = claims.Userid
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

		s, err := session.FromSessionID(sessionID, userid, c)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}
		sessionGob, err := s.ToGob()
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}
		if err := ch.HSet(s.UserKey(), s.SessionID, sessionGob).Err(); err != nil {
			return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
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
		userid := ""
		// if session_id is provided, is in cache, and is valid, set it as the sessionID
		// the session cannot be expired
		if ok, claims := u.tokenizer.GetClaims(ruser.RefreshToken, refreshSubject); ok {
			if s := strings.Split(claims.Id, ":"); len(s) == 2 {
				if key, err := ch.Get(s[0]).Result(); err == nil {
					sessionID = s[0]
					sessionKey = key
					userid = claims.Userid
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
		refreshToken, err := u.tokenizer.GenerateFromClaims(claims, u.refreshTime, refreshSubject, sessionID+":"+sessionKey)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}

		// generate a new sessionToken from the refreshToken claims
		sessionToken, err := u.tokenizer.GenerateFromClaims(claims, u.refreshTime, sessionSubject, sessionID)
		if err != nil {
			err.AddTrace(moduleIDAuth)
			return err
		}

		// set the session id and key into cache
		if err := ch.Set(sessionID, sessionKey, time.Duration(u.refreshTime*b1)).Err(); err != nil {
			return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
		}

		u.setRefreshCookie(c, conf, refreshToken, claims.AuthTags)
		u.setSessionCookie(c, conf, sessionToken, userid)

		return c.JSON(http.StatusOK, resUserAuth{
			Valid:        true,
			RefreshToken: refreshToken,
		})
	})

	r.POST("/logout", func(c echo.Context) error {
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
		// if session_id is provided, is in cache, and is valid, set it as the sessionID
		// the session can be expired by time
		if ok, claims := u.tokenizer.GetClaims(ruser.RefreshToken, refreshSubject); ok {
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
		validToken, _ := u.tokenizer.ValidateSkipTime(ruser.RefreshToken, refreshSubject, sessionID+":"+sessionKey)
		if !validToken {
			return c.JSON(http.StatusUnauthorized, resUserAuth{
				Valid: false,
			})
		}

		// delete the session in cache
		if err := ch.Del(sessionID).Err(); err != nil {
			return governor.NewError(moduleIDAuth, err.Error(), 0, http.StatusInternalServerError)
		}

		rmAccessCookie(c, conf)
		rmRefreshCookie(c, conf)
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
