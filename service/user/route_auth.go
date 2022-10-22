package user

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

func (s *router) setAccessCookie(c governor.Context, accessToken string) {
	c.SetCookie(&http.Cookie{
		Name:     gate.CookieNameAccessToken,
		Value:    accessToken,
		Path:     s.s.baseURL,
		MaxAge:   int(s.s.authsettings.accessDuration/time.Second) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *router) setRefreshCookie(c governor.Context, refreshToken string, userid string) {
	maxage := int(s.s.authsettings.refreshDuration/time.Second) - 5
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     s.s.authURL,
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "refresh_token",
		Value:    refreshToken,
		Path:     s.s.authURL + "/id/" + userid,
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userid",
		Value:    userid,
		Path:     "/",
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "userid_" + userid,
		Value:    userid,
		Path:     "/",
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *router) setSessionCookie(c governor.Context, sessionID string, userid string) {
	c.SetCookie(&http.Cookie{
		Name:     "session_token_" + userid,
		Value:    sessionID,
		Path:     s.s.authURL,
		MaxAge:   int(s.s.authsettings.refreshDuration/time.Second) - 5,
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

func (s *router) rmAccessCookie(c governor.Context) {
	c.SetCookie(&http.Cookie{
		Name:   gate.CookieNameAccessToken,
		Value:  "invalid",
		MaxAge: -1,
		Path:   s.s.baseURL,
	})
}

func (s *router) rmRefreshCookie(c governor.Context, userid string) {
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   s.s.authURL,
	})
	c.SetCookie(&http.Cookie{
		Name:   "refresh_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   s.s.authURL + "/id/" + userid,
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
	//forge:valid
	reqUserAuth struct {
		Username     string `valid:"usernameOrEmail,has" json:"username"`
		Password     string `valid:"password,has" json:"password"`
		Code         string `valid:"OTPCode" json:"code"`
		Backup       string `valid:"OTPCode" json:"backup"`
		SessionToken string `valid:"sessionToken,has" json:"session_token"`
	}
)

func (r *reqUserAuth) validCode() error {
	if err := r.valid(); err != nil {
		return err
	}
	if len(r.Code) > 0 && len(r.Backup) > 0 {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "May not provide both otp code and backup code")
	}
	return nil
}

func (s *router) loginUser(c governor.Context) {
	var req reqUserAuth
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.validCode(); err != nil {
		c.WriteError(err)
		return
	}

	userid, err := s.s.getUseridForLogin(c.Ctx(), req.Username)
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

	var ipaddr string
	if ip := c.RealIP(); ip != nil {
		ipaddr = ip.String()
	}
	res, err := s.s.login(c.Ctx(), userid, req.Password, req.Code, req.Backup, req.SessionToken, ipaddr, c.Header("User-Agent"))
	if err != nil {
		c.WriteError(err)
		return
	}

	s.setAccessCookie(c, res.AccessToken)
	s.setRefreshCookie(c, res.RefreshToken, res.Claims.Subject)
	s.setSessionCookie(c, res.SessionID, res.Claims.Subject)

	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqRefreshToken struct {
		RefreshToken string `valid:"refreshToken,has" json:"refresh_token"`
	}
)

func (s *router) exchangeToken(c governor.Context) {
	var req reqRefreshToken
	if t, ok := getRefreshCookie(c); ok {
		req.RefreshToken = t
	} else if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	var ipaddr string
	if ip := c.RealIP(); ip != nil {
		ipaddr = ip.String()
	}
	res, err := s.s.exchangeToken(c.Ctx(), req.RefreshToken, ipaddr, c.Header("User-Agent"))
	if err != nil {
		c.WriteError(err)
		return
	}

	s.setAccessCookie(c, res.AccessToken)
	if res.Refresh {
		s.setRefreshCookie(c, res.RefreshToken, res.Claims.Subject)
		s.setSessionCookie(c, res.SessionID, res.Claims.Subject)
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) refreshToken(c governor.Context) {
	var req reqRefreshToken
	if t, ok := getRefreshCookie(c); ok {
		req.RefreshToken = t
	} else if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	var ipaddr string
	if ip := c.RealIP(); ip != nil {
		ipaddr = ip.String()
	}
	res, err := s.s.refreshToken(c.Ctx(), req.RefreshToken, ipaddr, c.Header("User-Agent"))
	if err != nil {
		if errors.Is(err, ErrorDiscardSession{}) && res != nil && res.Claims != nil && res.Claims.Subject != "" {
			s.rmAccessCookie(c)
			s.rmRefreshCookie(c, res.Claims.Subject)
		}
		c.WriteError(err)
		return
	}

	s.setAccessCookie(c, res.AccessToken)
	s.setRefreshCookie(c, res.RefreshToken, res.Claims.Subject)
	s.setSessionCookie(c, res.SessionID, res.Claims.Subject)

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) logoutUser(c governor.Context) {
	var req reqRefreshToken
	if t, ok := getRefreshCookie(c); ok {
		req.RefreshToken = t
	} else if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	userid, err := s.s.logout(c.Ctx(), req.RefreshToken)
	if err != nil {
		c.WriteError(err)
		return
	}

	s.rmAccessCookie(c)
	s.rmRefreshCookie(c, userid)

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountAuth(r governor.Router) {
	m := governor.NewMethodRouter(r)
	m.PostCtx("/login", s.loginUser)
	m.PostCtx("/exchange", s.exchangeToken)
	m.PostCtx("/refresh", s.refreshToken)
	m.PostCtx("/id/{id}/exchange", s.exchangeToken)
	m.PostCtx("/id/{id}/refresh", s.refreshToken)
	m.PostCtx("/logout", s.logoutUser)
}
