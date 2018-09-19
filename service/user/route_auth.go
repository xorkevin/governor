package user

import (
	"errors"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/token"
	"github.com/labstack/echo"
	"github.com/sirupsen/logrus"
	"net/http"
	"strings"
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

func (u *userRouter) setAccessCookie(c echo.Context, conf governor.Config, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     conf.BaseURL,
		MaxAge:   int(u.service.accessTime),
		HttpOnly: false,
	})
}

const (
	month6 = 43200 * 365
)

func (u *userRouter) setRefreshCookie(c echo.Context, conf governor.Config, refreshToken string, authTags string) {
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

func (u *userRouter) setSessionCookie(c echo.Context, conf governor.Config, sessionToken, userid string) {
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

func (u *userRouter) loginUser(c echo.Context) error {
	ruser := reqUserAuth{}
	if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
	}
	isEmail := false
	if err := ruser.validEmail(); err == nil {
		isEmail = true
	}

	userid := ""
	if isEmail {
		m, err := u.service.GetByEmail(ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDAuth)
			return err
		}
		userid = m.Userid
	} else {
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := u.service.GetByUsername(ruser.Username)
		if err != nil {
			if err.Code() == 2 {
				err.SetErrorUser()
			}
			err.AddTrace(moduleIDAuth)
			return err
		}
		userid = m.Userid
	}
	if t, err := getSessionCookie(c, userid); err == nil {
		ruser.SessionToken = t
	}

	ok, res, err := u.service.Login(userid, ruser.Password, ruser.SessionToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}
	if !ok {
		return c.JSON(http.StatusUnauthorized, res)
	}

	u.setAccessCookie(c, u.service.config, res.AccessToken)
	u.setRefreshCookie(c, u.service.config, res.RefreshToken, res.Claims.AuthTags)
	u.setSessionCookie(c, u.service.config, res.SessionToken, userid)

	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) exchangeToken(c echo.Context) error {
	ruser := reqExchangeToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	ok, res, err := u.service.ExchangeToken(ruser.RefreshToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}
	if !ok {
		return c.JSON(http.StatusUnauthorized, res)
	}

	u.setAccessCookie(c, u.service.config, res.AccessToken)
	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) refreshToken(c echo.Context, conf governor.Config, l *logrus.Logger) error {
	ruser := reqExchangeToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return governor.NewErrorUser(moduleIDAuth, err.Error(), 0, http.StatusBadRequest)
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	ok, res, err := u.service.RefreshToken(ruser.RefreshToken)
	if err != nil {
		return err
	}
	if !ok {
		return c.JSON(http.StatusUnauthorized, res)
	}

	u.setRefreshCookie(c, u.service.config, res.RefreshToken, res.Claims.AuthTags)
	u.setSessionCookie(c, u.service.config, res.SessionToken, res.Claims.Userid)
	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) logoutUser(c echo.Context, conf governor.Config, l *logrus.Logger) error {
	ch := u.service.cache.Cache()

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
	if ok, claims := u.service.tokenizer.GetClaims(ruser.RefreshToken, refreshSubject); ok {
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
	validToken, _ := u.service.tokenizer.ValidateSkipTime(ruser.RefreshToken, refreshSubject, sessionID+":"+sessionKey)
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
}

func (u *userRouter) decodeToken(c echo.Context, conf governor.Config, l *logrus.Logger) error {
	return c.JSON(http.StatusOK, resUserAuth{
		Valid:  true,
		Claims: c.Get("user").(*token.Claims),
	})
}

func (u *userRouter) mountAuth(conf governor.Config, r *echo.Group, l *logrus.Logger) error {
	r.POST("/login", u.loginUser)
	r.POST("/exchange", u.exchangeToken)

	r.POST("/refresh", func(c echo.Context) error {
		return u.refreshToken(c, conf, l)
	})

	r.POST("/logout", func(c echo.Context) error {
		return u.logoutUser(c, conf, l)
	})

	if conf.IsDebug() {
		r.GET("/decode", func(c echo.Context) error {
			return u.decodeToken(c, conf, l)
		}, gate.User(u.service.gate))
	}

	return nil
}
