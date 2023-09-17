package user

import (
	"errors"
	"net/http"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate"
)

func (s *router) setTokenCookies(c *governor.Context, accessToken, refreshToken string, userid, sessionID string) {
	c.SetCookie(&http.Cookie{
		Name:     gate.CookieNameAccessToken,
		Value:    accessToken,
		Path:     s.s.baseURL,
		MaxAge:   int(s.s.authSettings.accessDuration/time.Second) - 5,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	maxage := int(s.s.authSettings.refreshDuration/time.Second) - 5
	c.SetCookie(&http.Cookie{
		Name:     "gov_refresh",
		Value:    refreshToken,
		Path:     s.s.authURL,
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "gov_refresh",
		Value:    refreshToken,
		Path:     s.s.authURL + "/id/" + userid,
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "gov_userid",
		Value:    userid,
		Path:     "/",
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
	c.SetCookie(&http.Cookie{
		Name:     "gov_userid_" + userid,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxage,
		HttpOnly: false,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *router) rmTokenCookies(c *governor.Context, userid string) {
	c.SetCookie(&http.Cookie{
		Name:   gate.CookieNameAccessToken,
		Value:  "invalid",
		MaxAge: -1,
		Path:   s.s.baseURL,
	})
	c.SetCookie(&http.Cookie{
		Name:   "gov_refresh",
		Value:  "invalid",
		MaxAge: -1,
		Path:   s.s.authURL,
	})
	c.SetCookie(&http.Cookie{
		Name:   "gov_refresh",
		Value:  "invalid",
		MaxAge: -1,
		Path:   s.s.authURL + "/id/" + userid,
	})
	c.SetCookie(&http.Cookie{
		Name:   "gov_userid",
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
	c.SetCookie(&http.Cookie{
		Name:   "gov_userid_" + userid,
		Value:  "invalid",
		MaxAge: -1,
		Path:   "/",
	})
}

func getRefreshCookie(c *governor.Context) (string, bool) {
	cookie, err := c.Cookie("gov_refresh")
	if err != nil {
		return "", false
	}
	if cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

func getSessionCookie(c *governor.Context, userid string) (string, bool) {
	if userid == "" {
		return "", false
	}
	cookie, err := c.Cookie("gov_userid_" + userid)
	if err != nil {
		return "", false
	}
	if cookie.Value == "" {
		return "", false
	}
	return cookie.Value, true
}

type (
	//forge:valid
	reqUserAuth struct {
		Username  string `valid:"username,opt" json:"username"`
		Email     string `valid:"email,opt" json:"email"`
		Password  string `valid:"password,has" json:"password"`
		Code      string `valid:"OTPCode,opt" json:"code"`
		Backup    string `valid:"OTPCode,opt" json:"backup"`
		SessionID string `valid:"sessionID,opt" json:"session_id"`
	}
)

func (r *reqUserAuth) preValidate() error {
	if err := r.valid(); err != nil {
		return err
	}
	if r.Username == "" && r.Email == "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "Must provide either username and email")
	}
	if r.Username != "" && r.Email != "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "May not provide both username and email")
	}
	if r.Code != "" && r.Backup != "" {
		return governor.ErrWithRes(nil, http.StatusBadRequest, "", "May not provide both otp code and backup code")
	}
	return nil
}

func (s *router) loginUser(c *governor.Context) {
	var req reqUserAuth
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(err)
		return
	}
	if err := req.preValidate(); err != nil {
		c.WriteError(err)
		return
	}

	userid, err := s.s.getUseridForLogin(c.Ctx(), req.Username, req.Email)
	if err != nil {
		c.WriteError(err)
		return
	}
	if t, ok := getSessionCookie(c, userid); ok {
		req.SessionID = t
	}

	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	var ipaddr string
	if ip := c.RealIP(); ip != nil {
		ipaddr = ip.String()
	}
	res, err := s.s.login(c.Ctx(), userid, req.Password, req.Code, req.Backup, req.SessionID, ipaddr, c.Header("User-Agent"))
	if err != nil {
		c.WriteError(err)
		return
	}

	s.setTokenCookies(c, res.AccessToken, res.RefreshToken, res.Claims.Subject, res.Claims.SessionID)

	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqRefreshToken struct {
		RefreshToken string `valid:"refreshToken,has" json:"refresh_token"`
	}
)

func (s *router) refreshToken(c *governor.Context) {
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
		if errors.Is(err, errDiscardSession{}) && res != nil && res.Claims != nil && res.Claims.Subject != "" {
			s.rmTokenCookies(c, res.Claims.Subject)
		}
		c.WriteError(err)
		return
	}

	s.setTokenCookies(c, res.AccessToken, res.RefreshToken, res.Claims.Subject, res.Claims.SessionID)

	c.WriteJSON(http.StatusOK, res)
}

func (s *router) logoutUser(c *governor.Context) {
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
		if errors.Is(err, errDiscardSession{}) && userid != "" {
			s.rmTokenCookies(c, userid)
		}
		c.WriteError(err)
		return
	}

	s.rmTokenCookies(c, userid)

	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountAuth(r governor.Router) {
	m := governor.NewMethodRouter(r)
	m.PostCtx("/login", s.loginUser)
	m.PostCtx("/refresh", s.refreshToken)
	m.PostCtx("/logout", s.logoutUser)
	m.PostCtx("/id/{id}/refresh", s.refreshToken)
	m.PostCtx("/id/{id}/logout", s.logoutUser)
}
