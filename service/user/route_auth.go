package user

import (
	"net"
	"net/http"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/ratelimit"
)

//go:generate forge validation -o validation_auth_gen.go reqUserAuth reqRefreshToken

func (m *router) setAccessCookie(c governor.Context, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     "access_token",
		Value:    accessToken,
		Path:     m.s.baseURL,
		MaxAge:   int(m.s.accessTime) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *router) setRefreshCookie(c governor.Context, refreshToken string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     m.s.authURL,
		MaxAge:   int(m.s.refreshTime) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     m.s.authURL + "/id/" + userid,
		MaxAge:   int(m.s.refreshTime) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userid",
		Value:    userid,
		Path:     "/",
		MaxAge:   int(m.s.refreshTime) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userid_" + userid,
		Value:    userid,
		Path:     "/",
		MaxAge:   int(m.s.refreshTime) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *router) setSessionCookie(c governor.Context, sessionToken string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "session_token_" + userid,
		Value:    sessionToken,
		Path:     m.s.authURL,
		MaxAge:   int(m.s.refreshTime),
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func getRefreshCookie(c governor.Context) (string, bool) {
	cookie, err := c.Cookie("refresh_token")
	if err != nil {
		return "", false
	}
	if cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func getSessionCookie(c governor.Context, userid string) (string, bool) {
	if userid == "" {
		return "", false
	}
	cookie, err := c.Cookie("session_token_" + userid)
	if err != nil {
		return "", false
	}
	if cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func (m *router) rmAccessCookie(c governor.Context) {
	c.SetCookie(&http.Cookie{
		Name:   "access_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   m.s.baseURL,
	})
}

func (m *router) rmRefreshCookie(c governor.Context, userid string) {
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   m.s.authURL,
	})
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   m.s.authURL + "/id/" + userid,
	})
	c.SetCookie(&http.Cookie{
		Name:   "userid",
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
	c.SetCookie(&http.Cookie{
		Name:   "userid_" + userid,
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
}

type (
	reqUserAuth struct {
		Username     string `valid:"usernameOrEmail,has" json:"username"`
		Password     string `valid:"password,has" json:"password"`
		SessionToken string `valid:"sessionToken,has" json:"session_token"`
	}
)

func getHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (m *router) loginUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqUserAuth{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	userid, err := m.s.GetUseridForLogin(req.Username)
	if err != nil {
		c.WriteError(err)
		return
	}
	if t, ok := getSessionCookie(c, userid); ok {
		req.SessionToken = t
	}

	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.Login(userid, req.Password, req.SessionToken, getHost(r), c.Header("User-Agent"))
	if err != nil {
		c.WriteError(err)
		return
	}

	m.setAccessCookie(c, res.AccessToken)
	m.setRefreshCookie(c, res.RefreshToken, res.Claims.Subject)
	m.setSessionCookie(c, res.SessionToken, res.Claims.Subject)

	c.WriteJSON(http.StatusOK, res)
}

type (
	reqRefreshToken struct {
		RefreshToken string `valid:"refreshToken,has" json:"refresh_token"`
	}
)

func (m *router) exchangeToken(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	ruser := reqRefreshToken{}
	if t, ok := getRefreshCookie(c); ok {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		c.WriteError(err)
		return
	}
	if err := ruser.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.ExchangeToken(ruser.RefreshToken, getHost(r), c.Header("User-Agent"))
	if err != nil {
		c.WriteError(err)
		return
	}

	m.setAccessCookie(c, res.AccessToken)
	if res.Refresh {
		m.setRefreshCookie(c, res.RefreshToken, res.Claims.Subject)
		m.setSessionCookie(c, res.SessionToken, res.Claims.Subject)
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) refreshToken(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	ruser := reqRefreshToken{}
	if t, ok := getRefreshCookie(c); ok {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		c.WriteError(err)
		return
	}
	if err := ruser.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.RefreshToken(ruser.RefreshToken, getHost(r), c.Header("User-Agent"))
	if err != nil {
		c.WriteError(err)
		return
	}

	m.setAccessCookie(c, res.AccessToken)
	m.setRefreshCookie(c, res.RefreshToken, res.Claims.Subject)
	m.setSessionCookie(c, res.SessionToken, res.Claims.Subject)

	c.WriteJSON(http.StatusOK, res)
}

func (m *router) logoutUser(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	ruser := reqRefreshToken{}
	if t, ok := getRefreshCookie(c); ok {
		ruser.RefreshToken = t
	} else if err := c.Bind(&ruser); err != nil {
		c.WriteError(err)
		return
	}
	if err := ruser.valid(); err != nil {
		c.WriteError(err)
		return
	}

	userid, err := m.s.Logout(ruser.RefreshToken)
	if err != nil {
		c.WriteError(err)
		return
	}

	m.rmAccessCookie(c)
	m.rmRefreshCookie(c, userid)

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountAuth(r governor.Router) {
	rt := ratelimit.Compose(m.s.ratelimiter, ratelimit.IPAddress("ip", 60, 15, 240))
	r.Post("/login", m.loginUser, rt)
	r.Post("/exchange", m.exchangeToken, rt)
	r.Post("/refresh", m.refreshToken, rt)
	r.Post("/id/{id}/exchange", m.exchangeToken, rt)
	r.Post("/id/{id}/refresh", m.refreshToken, rt)
	r.Post("/logout", m.logoutUser, rt)
}
