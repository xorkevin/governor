package user

import (
	"errors"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/token"
	"github.com/labstack/echo"
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

func (r *reqUserAuth) valid() error {
	if err := hasUsername(r.Username); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r *reqUserAuth) validEmail() error {
	if err := validEmail(r.Username); err != nil {
		return err
	}
	if err := hasPassword(r.Password); err != nil {
		return err
	}
	return nil
}

func (r *reqExchangeToken) valid() error {
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
		return err
	}
	isEmail := false
	if err := ruser.validEmail(); err == nil {
		isEmail = true
	}

	userid := ""
	if isEmail {
		m, err := u.service.GetByEmail(ruser.Username)
		if err != nil {
			return err
		}
		userid = m.Userid
	} else {
		if err := ruser.valid(); err != nil {
			return err
		}
		m, err := u.service.GetByUsername(ruser.Username)
		if err != nil {
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
		return err
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

func (u *userRouter) refreshToken(c echo.Context) error {
	ruser := reqExchangeToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
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

func (u *userRouter) logoutUser(c echo.Context) error {
	ruser := reqExchangeToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	ok, err := u.service.Logout(ruser.RefreshToken)
	if err != nil {
		return err
	}
	if !ok {
		return c.JSON(http.StatusUnauthorized, resUserAuth{
			Valid: false,
		})
	}

	rmAccessCookie(c, u.service.config)
	rmRefreshCookie(c, u.service.config)
	return c.NoContent(http.StatusNoContent)
}

func (u *userRouter) decodeToken(c echo.Context) error {
	return c.JSON(http.StatusOK, resUserAuth{
		Valid:  true,
		Claims: c.Get("user").(*token.Claims),
	})
}

func (u *userRouter) mountAuth(conf governor.Config, r *echo.Group) error {
	r.POST("/login", u.loginUser)
	r.POST("/exchange", u.exchangeToken)
	r.POST("/refresh", u.refreshToken)
	r.POST("/logout", u.logoutUser)
	if conf.IsDebug() {
		r.GET("/decode", u.decodeToken, gate.User(u.service.gate))
	}
	return nil
}
