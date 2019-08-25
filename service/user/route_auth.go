package user

import (
	"errors"
	"github.com/labstack/echo"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
)

//go:generate forge validation -o validation_auth_gen.go reqUserAuth reqRefreshToken

func (u *userRouter) setAccessCookie(c echo.Context, conf governor.Config, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     conf.BaseURL,
		MaxAge:   int(u.service.accessTime) - 5,
		HttpOnly: false,
	})
}

func (u *userRouter) setRefreshCookie(c echo.Context, conf governor.Config, refreshToken string, authTags string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     conf.BaseURL + "/u/auth",
		MaxAge:   int(u.service.refreshTime),
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "refresh_valid",
		Value:    "valid",
		Path:     "/",
		MaxAge:   int(u.service.refreshTime),
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "auth_tags",
		Value:    authTags,
		Path:     "/",
		MaxAge:   int(u.service.refreshTime),
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userid",
		Value:    userid,
		Path:     "/",
		MaxAge:   int(u.service.refreshTime),
		HttpOnly: false,
	})
}

func (u *userRouter) setSessionCookie(c echo.Context, conf governor.Config, sessionToken string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "session_token_" + userid,
		Value:    sessionToken,
		Path:     conf.BaseURL + "/u/auth/login",
		MaxAge:   int(u.service.refreshTime),
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
	if userid == "" {
		return "", errors.New("no cookie value")
	}
	cookie, err := c.Cookie("session_token_" + userid)
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
	c.SetCookie(&http.Cookie{
		Name:   "userid",
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
}

func rmSessionCookie(c echo.Context, conf governor.Config, userid string) {
	c.SetCookie(&http.Cookie{
		Name:   "session_token_" + userid,
		Value:  "invalid",
		MaxAge: -1,
		Path:   conf.BaseURL + "/u/auth/login",
	})
}

type (
	reqUserAuth struct {
		Username     string `json:"username"`
		Password     string `valid:"password,has" json:"password"`
		SessionToken string `valid:"sessionToken,has" json:"session_token"`
	}
)

func (r *reqUserAuth) validIsEmail() (bool, error) {
	return validhasUsernameOrEmail(r.Username)
}

func (u *userRouter) loginUser(c echo.Context) error {
	ruser := reqUserAuth{}
	if err := c.Bind(&ruser); err != nil {
		return err
	}
	isEmail, err := ruser.validIsEmail()
	if err != nil {
		return err
	}

	userid := ""
	if isEmail {
		m, err := u.service.GetByEmail(ruser.Username)
		if err != nil {
			return err
		}
		userid = m.Userid
	} else {
		m, err := u.service.GetByUsername(ruser.Username)
		if err != nil {
			return err
		}
		userid = m.Userid
	}
	if t, err := getSessionCookie(c, userid); err == nil {
		ruser.SessionToken = t
	}

	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.Login(userid, ruser.Password, ruser.SessionToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}

	u.setAccessCookie(c, u.service.config, res.AccessToken)
	u.setRefreshCookie(c, u.service.config, res.RefreshToken, res.Claims.AuthTags, res.Claims.Userid)
	u.setSessionCookie(c, u.service.config, res.SessionToken, res.Claims.Userid)

	return c.JSON(http.StatusOK, res)
}

type (
	reqRefreshToken struct {
		RefreshToken string `valid:"refreshToken,has" json:"refresh_token"`
	}
)

func (u *userRouter) exchangeToken(c echo.Context) error {
	ruser := reqRefreshToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.ExchangeToken(ruser.RefreshToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}

	u.setAccessCookie(c, u.service.config, res.AccessToken)
	if len(res.RefreshToken) > 0 {
		u.setRefreshCookie(c, u.service.config, res.RefreshToken, res.Claims.AuthTags, res.Claims.Userid)
	}
	if len(res.SessionToken) > 0 {
		u.setSessionCookie(c, u.service.config, res.SessionToken, res.Claims.Userid)
	}
	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) refreshToken(c echo.Context) error {
	ruser := reqRefreshToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := u.service.RefreshToken(ruser.RefreshToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}

	u.setAccessCookie(c, u.service.config, res.AccessToken)
	u.setRefreshCookie(c, u.service.config, res.RefreshToken, res.Claims.AuthTags, res.Claims.Userid)
	u.setSessionCookie(c, u.service.config, res.SessionToken, res.Claims.Userid)
	return c.JSON(http.StatusOK, res)
}

func (u *userRouter) logoutUser(c echo.Context) error {
	ruser := reqRefreshToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	err := u.service.Logout(ruser.RefreshToken)
	if err != nil {
		return err
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
