package oauth

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/kerrors"
)

func (s *router) getOpenidConfig(c *governor.Context) {
	res, err := s.s.getOpenidConfig()
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) getJWKS(c *governor.Context) {
	res, err := s.s.getJWKS(c.Ctx())
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqOAuthAuthorize struct {
		Userid              string `valid:"userid,has" json:"-"`
		ClientID            string `valid:"clientID,has" json:"client_id"`
		Scope               string `valid:"oidScope" json:"scope"`
		Nonce               string `valid:"oidNonce" json:"nonce"`
		CodeChallenge       string `valid:"oidCodeChallenge" json:"code_challenge"`
		CodeChallengeMethod string `valid:"oidCodeChallengeMethod" json:"code_challenge_method"`
	}
)

func (s *router) authCode(c *governor.Context) {
	claims := gate.GetCtxClaims(c)
	if claims == nil {
		c.WriteError(governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "Unauthorized"))
		return
	}
	var req reqOAuthAuthorize
	if err := c.Bind(&req, false); err != nil {
		c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, oidErrorInvalidRequest, "Unauthorized"))
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(governor.ErrWithRes(err, http.StatusBadRequest, oidErrorInvalidRequest, "Unauthorized"))
		return
	}
	if req.CodeChallengeMethod == "" && req.CodeChallenge != "" {
		c.WriteError(governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "No code challenge method provided"))
		return
	}
	if req.CodeChallengeMethod != "" && req.CodeChallenge == "" {
		c.WriteError(governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "No code challenge provided"))
		return
	}

	res, err := s.s.authCode(c.Ctx(), req.Userid, req.ClientID, req.Scope, req.Nonce, req.CodeChallenge, req.CodeChallengeMethod, claims.AuthTime)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqOAuthTokenCode struct {
		ClientID     string `valid:"oidClientID,has" json:"-"`
		ClientSecret string `valid:"oidClientSecret,has" json:"-"`
		RedirectURI  string `valid:"oidRedirect,has" json:"-"`
		Userid       string `valid:"oidUserid,has" json:"-"`
		Code         string `valid:"oidCode,has" json:"-"`
		CodeVerifier string `valid:"oidCodeVerifier,opt" json:"-"`
	}

	resAuthTokenErr struct {
		Error string `json:"error"`
		Desc  string `json:"error_description,omitempty"`
		URI   string `json:"error_uri,omitempty"`
	}
)

func (s *router) writeOAuthTokenError(c *governor.Context, err error) {
	var rerr *governor.ErrorRes
	if !errors.As(err, &rerr) {
		rerr = &governor.ErrorRes{
			Status:  http.StatusInternalServerError,
			Code:    oidErrorServer,
			Message: "Internal Server Error",
		}
	}

	if !errors.Is(err, governor.ErrorNoLog) {
		if rerr.Status >= http.StatusBadRequest && rerr.Status < http.StatusInternalServerError {
			s.s.log.WarnErr(c.Ctx(), err, nil)
		} else {
			s.s.log.Err(c.Ctx(), err, nil)
		}
	}

	if rerr.Status == http.StatusUnauthorized {
		c.SetHeader("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, s.s.realm))
	}
	c.WriteJSON(rerr.Status, resAuthTokenErr{
		Error: rerr.Code,
		Desc:  rerr.Message,
	})
}

func (s *router) authToken(c *governor.Context) {
	grantType := c.FormValue("grant_type")
	if err := validOidGrantType(grantType); err != nil {
		s.writeOAuthTokenError(c, err)
		return
	}
	if grantType == oidGrantTypeCode {
		req := reqOAuthTokenCode{
			ClientID:     c.FormValue("client_id"),
			ClientSecret: c.FormValue("client_secret"),
			RedirectURI:  c.FormValue("redirect_uri"),
			CodeVerifier: c.FormValue("code_verifier"),
		}
		if user, pass, ok := c.BasicAuth(); ok {
			if req.ClientID != "" || req.ClientSecret != "" {
				s.writeOAuthTokenError(c, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "Client secret basic and post used"))
				return
			}
			var err error
			req.ClientID, err = url.QueryUnescape(user)
			if err != nil {
				s.writeOAuthTokenError(c, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "Invalid client id encoding"))
				return
			}
			req.ClientSecret, err = url.QueryUnescape(pass)
			if err != nil {
				s.writeOAuthTokenError(c, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "Invalid client secret encoding"))
				return
			}
		}
		if userid, code, ok := strings.Cut(c.FormValue("code"), keySeparator); ok {
			req.Userid = userid
			req.Code = code
		}
		if err := req.valid(); err != nil {
			s.writeOAuthTokenError(c, err)
			return
		}
		res, err := s.s.authTokenCode(c.Ctx(), req.ClientID, req.ClientSecret, req.RedirectURI, req.Userid, req.Code, req.CodeVerifier)
		if err != nil {
			s.writeOAuthTokenError(c, err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
		return
	}
	s.writeOAuthTokenError(c, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorUnsupportedGrant, "Unsupported grant type"))
	return
}

func (s *router) userinfo(c *governor.Context) {
	claims := gate.GetCtxClaims(c)
	if claims == nil {
		c.WriteError(kerrors.WithMsg(nil, "No access token claims"))
		return
	}
	res, err := s.s.userinfo(c.Ctx(), claims.Subject, claims.Scope)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetConnectionGroup struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (s *router) getConnections(c *governor.Context) {
	req := reqGetConnectionGroup{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getConnections(c.Ctx(), req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	//forge:valid
	reqGetConnection struct {
		Userid   string `valid:"userid,has" json:"-"`
		ClientID string `valid:"clientID,has" json:"-"`
	}
)

func (s *router) getConnection(c *governor.Context) {
	req := reqGetConnection{
		Userid:   gate.GetCtxUserid(c),
		ClientID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := s.s.getConnection(c.Ctx(), req.Userid, req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (s *router) delConnection(c *governor.Context) {
	req := reqGetConnection{
		Userid:   gate.GetCtxUserid(c),
		ClientID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := s.s.delConnection(c.Ctx(), req.Userid, req.ClientID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (s *router) mountOidRoutes(r governor.Router) {
	m := governor.NewMethodRouter(r)
	scopeConnectionRead := s.s.scopens + ".connection:read"
	scopeConnectionWrite := s.s.scopens + ".connection:write"
	m.GetCtx("/openid-configuration", s.getOpenidConfig, s.rt)
	m.GetCtx(jwksRoute, s.getJWKS, s.rt)
	m.PostCtx("/auth/code", s.authCode, gate.User(s.s.gate, token.ScopeForbidden), s.rt)
	m.PostCtx(tokenRoute, s.authToken, cachecontrol.ControlNoStoreCtx, s.rt)
	m.GetCtx(userinfoRoute, s.userinfo, gate.User(s.s.gate, oidScopeOpenid), s.rt)
	m.PostCtx(userinfoRoute, s.userinfo, gate.User(s.s.gate, oidScopeOpenid), s.rt)
	m.GetCtx("/connection", s.getConnections, gate.User(s.s.gate, scopeConnectionRead), s.rt)
	m.GetCtx("/connection/id/{id}", s.getConnection, gate.User(s.s.gate, scopeConnectionRead), s.rt)
	m.DeleteCtx("/connection/id/{id}", s.delConnection, gate.User(s.s.gate, scopeConnectionWrite), s.rt)
}
