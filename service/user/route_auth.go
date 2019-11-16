package user

import (
	"errors"
	"github.com/labstack/echo/v4"
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
)

//go:generate forge validation -o validation_auth_gen.go reqUserAuth reqRefreshToken

func (r *router) setAccessCookie(c echo.Context, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     r.s.baseURL,
		MaxAge:   int(r.s.accessTime) - 5,
		HttpOnly: false,
	})
}

func (r *router) setRefreshCookie(c echo.Context, refreshToken string, authTags string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     r.s.authURL,
		MaxAge:   int(r.s.refreshTime),
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "refresh_valid",
		Value:    "valid",
		Path:     "/",
		MaxAge:   int(r.s.refreshTime),
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "auth_tags",
		Value:    authTags,
		Path:     "/",
		MaxAge:   int(r.s.refreshTime),
		HttpOnly: false,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userid",
		Value:    userid,
		Path:     "/",
		MaxAge:   int(r.s.refreshTime),
		HttpOnly: false,
	})
}

func (r *router) setSessionCookie(c echo.Context, sessionToken string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "session_token_" + userid,
		Value:    sessionToken,
		Path:     r.s.authURL,
		MaxAge:   int(r.s.refreshTime),
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

func (r *router) rmAccessCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:   "access_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   r.s.baseURL,
	})
}

func (r *router) rmRefreshCookie(c echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   r.s.authURL,
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

func (r *router) rmSessionCookie(c echo.Context, conf governor.Config, userid string) {
	c.SetCookie(&http.Cookie{
		Name:   "session_token_" + userid,
		Value:  "invalid",
		MaxAge: -1,
		Path:   r.s.authURL,
	})
}

type (
	reqUserAuth struct {
		Username     string `valid:"usernameOrEmail,has" json:"username"`
		Password     string `valid:"password,has" json:"password"`
		SessionToken string `valid:"sessionToken,has" json:"session_token"`
	}
)

func (r *router) loginUser(c echo.Context) error {
	req := reqUserAuth{}
	if err := c.Bind(&req); err != nil {
		return err
	}
	if err := req.valid(); err != nil {
		return err
	}

	userid := ""
	if isEmail(req.Username) {
		m, err := r.s.GetByEmail(req.Username)
		if err != nil {
			return err
		}
		userid = m.Userid
	} else {
		m, err := r.s.GetByUsername(req.Username)
		if err != nil {
			return err
		}
		userid = m.Userid
	}
	if t, err := getSessionCookie(c, userid); err == nil {
		req.SessionToken = t
	}

	if err := req.valid(); err != nil {
		return err
	}

	res, err := r.s.Login(userid, req.Password, req.SessionToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}

	r.setAccessCookie(c, res.AccessToken)
	r.setRefreshCookie(c, res.RefreshToken, res.Claims.AuthTags, res.Claims.Userid)
	r.setSessionCookie(c, res.SessionToken, res.Claims.Userid)

	return c.JSON(http.StatusOK, res)
}

type (
	reqRefreshToken struct {
		RefreshToken string `valid:"refreshToken,has" json:"refresh_token"`
	}
)

func (r *router) exchangeToken(c echo.Context) error {
	ruser := reqRefreshToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.ExchangeToken(ruser.RefreshToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}

	r.setAccessCookie(c, res.AccessToken)
	if len(res.RefreshToken) > 0 {
		r.setRefreshCookie(c, res.RefreshToken, res.Claims.AuthTags, res.Claims.Userid)
	}
	if len(res.SessionToken) > 0 {
		r.setSessionCookie(c, res.SessionToken, res.Claims.Userid)
	}
	return c.JSON(http.StatusOK, res)
}

func (r *router) refreshToken(c echo.Context) error {
	ruser := reqRefreshToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	res, err := r.s.RefreshToken(ruser.RefreshToken, c.RealIP(), c.Request().Header.Get("User-Agent"))
	if err != nil {
		return err
	}

	r.setAccessCookie(c, res.AccessToken)
	r.setRefreshCookie(c, res.RefreshToken, res.Claims.AuthTags, res.Claims.Userid)
	r.setSessionCookie(c, res.SessionToken, res.Claims.Userid)
	return c.JSON(http.StatusOK, res)
}

func (r *router) logoutUser(c echo.Context) error {
	ruser := reqRefreshToken{}
	if t, err := getRefreshCookie(c); err == nil {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		return err
	}
	if err := ruser.valid(); err != nil {
		return err
	}

	err := r.s.Logout(ruser.RefreshToken)
	if err != nil {
		return err
	}

	r.rmAccessCookie(c)
	r.rmRefreshCookie(c)
	return c.NoContent(http.StatusNoContent)
}

func (r *router) decodeToken(c echo.Context) error {
	return c.JSON(http.StatusOK, resUserAuth{
		Valid:  true,
		Claims: c.Get("user").(*token.Claims),
	})
}

func (r *router) mountAuth(debugMode bool, g *echo.Group) error {
	g.POST("/login", r.loginUser)
	g.POST("/exchange", r.exchangeToken)
	g.POST("/refresh", r.refreshToken)
	g.POST("/logout", r.logoutUser)
	if debugMode {
		g.GET("/decode", r.decodeToken, gate.User(r.s.gate))
	}
	return nil
}
