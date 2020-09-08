package user

import (
	"net"
	"net/http"
	"xorkevin.dev/governor"
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
		Name:     "userid",
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

func getAccessCookie(c governor.Context) (string, bool) {
	cookie, err := c.Cookie("access_token")
	if err != nil {
		return "", false
	}
	if cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
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

func (m *router) rmRefreshCookie(c governor.Context) {
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   m.s.authURL,
	})
	c.SetCookie(&http.Cookie{
		Name:   "userid",
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
}

func (m *router) rmSessionCookie(c governor.Context, userid string) {
	c.SetCookie(&http.Cookie{
		Name:   "session_token_" + userid,
		Value:  "invalid",
		MaxAge: -1,
		Path:   m.s.authURL,
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

	userid := ""
	if isEmail(req.Username) {
		m, err := m.s.GetByEmail(req.Username)
		if err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
				c.WriteError(governor.NewErrorUser("Invalid username or password", http.StatusUnauthorized, nil))
				return
			}
			c.WriteError(err)
			return
		}
		userid = m.Userid
	} else {
		m, err := m.s.GetByUsername(req.Username)
		if err != nil {
			if governor.ErrorStatus(err) == http.StatusNotFound {
				c.WriteError(governor.NewErrorUser("Invalid username or password", http.StatusUnauthorized, nil))
				return
			}
			c.WriteError(err)
			return
		}
		userid = m.Userid
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

	if err := m.s.Logout(ruser.RefreshToken); err != nil {
		c.WriteError(err)
		return
	}

	m.rmAccessCookie(c)
	m.rmRefreshCookie(c)

	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountAuth(r governor.Router) {
	r.Post("/login", m.loginUser)
	r.Post("/exchange", m.exchangeToken)
	r.Post("/refresh", m.refreshToken)
	r.Post("/logout", m.logoutUser)
}
