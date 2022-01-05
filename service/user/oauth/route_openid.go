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
)

//go:generate forge validation -o validation_openid_gen.go reqOAuthAuthorize reqOAuthTokenCode reqGetConnectionGroup reqGetConnection

func (m *router) getOpenidConfig(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	res, err := m.s.GetOpenidConfig()
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) getJWKS(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	res, err := m.s.GetJWKS()
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqOAuthAuthorize struct {
		Userid              string `valid:"userid,has" json:"-"`
		ClientID            string `valid:"clientID,has" json:"client_id"`
		Scope               string `valid:"oidScope" json:"scope"`
		Nonce               string `valid:"oidNonce" json:"nonce"`
		CodeChallenge       string `valid:"oidCodeChallenge" json:"code_challenge"`
		CodeChallengeMethod string `valid:"oidCodeChallengeMethod" json:"code_challenge_method"`
	}
)

func (m *router) authCode(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	claims := gate.GetCtxClaims(c)
	if claims == nil {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Unauthorized",
		})))
		return
	}
	req := reqOAuthAuthorize{}
	if err := c.Bind(&req); err != nil {
		c.WriteError(err)
		return
	}
	req.Userid = gate.GetCtxUserid(c)
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}
	if req.CodeChallengeMethod == "" && req.CodeChallenge != "" {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "No code challenge method provided",
		})))
		return
	}
	if req.CodeChallengeMethod != "" && req.CodeChallenge == "" {
		c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "No code challenge provided",
		})))
		return
	}

	res, err := m.s.AuthCode(req.Userid, req.ClientID, req.Scope, req.Nonce, req.CodeChallenge, req.CodeChallengeMethod, claims.AuthTime)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
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

func (m *router) writeOAuthTokenError(c governor.Context, err error) {
	var gerr *governor.Error
	isError := errors.As(err, &gerr)

	if !errors.Is(err, governor.ErrorUser{}) {
		msg := "non governor error"
		if isError {
			msg = gerr.Message
		}
		m.s.logger.Error(msg, map[string]string{
			"endpoint": c.Req().URL.EscapedPath(),
			"error":    err.Error(),
		})
	}

	var rerr *governor.ErrorRes
	if errors.As(err, &rerr) {
		if rerr.Status == http.StatusUnauthorized {
			c.SetHeader("WWW-Authenticate", fmt.Sprintf(`Basic realm="%s"`, m.s.realm))
		}
		c.WriteJSON(rerr.Status, resAuthTokenErr{
			Error: rerr.Code,
			Desc:  rerr.Message,
		})
		return
	}

	msg := "Internal Server Error"
	if isError {
		msg = gerr.Message
	}
	c.WriteJSON(http.StatusInternalServerError, resAuthTokenErr{
		Error: oidErrorServer,
		Desc:  msg,
	})
}

func (m *router) authToken(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	grantType := c.FormValue("grant_type")
	if err := validOidGrantType(grantType); err != nil {
		m.writeOAuthTokenError(c, err)
		return
	}
	if grantType == oidGrantTypeCode {
		req := reqOAuthTokenCode{
			ClientID:     c.FormValue("client_id"),
			ClientSecret: c.FormValue("client_secret"),
			RedirectURI:  c.FormValue("redirect_uri"),
			CodeVerifier: c.FormValue("code_verifier"),
		}
		if user, pass, ok := r.BasicAuth(); ok {
			if req.ClientID != "" || req.ClientSecret != "" {
				m.writeOAuthTokenError(c, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    oidErrorInvalidRequest,
					Message: "Client secret basic and post used",
				})))
				return
			}
			var err error
			req.ClientID, err = url.QueryUnescape(user)
			if err != nil {
				m.writeOAuthTokenError(c, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    oidErrorInvalidRequest,
					Message: "Invalid client id encoding",
				})))
				return
			}
			req.ClientSecret, err = url.QueryUnescape(pass)
			if err != nil {
				m.writeOAuthTokenError(c, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusBadRequest,
					Code:    oidErrorInvalidRequest,
					Message: "Invalid client secret encoding",
				})))
				return
			}
		}
		if j := strings.SplitN(c.FormValue("code"), keySeparator, 2); len(j) == 2 {
			req.Userid = j[0]
			req.Code = j[1]
		}
		if err := req.valid(); err != nil {
			m.writeOAuthTokenError(c, err)
			return
		}
		res, err := m.s.AuthTokenCode(req.ClientID, req.ClientSecret, req.RedirectURI, req.Userid, req.Code, req.CodeVerifier)
		if err != nil {
			m.writeOAuthTokenError(c, err)
			return
		}
		c.WriteJSON(http.StatusOK, res)
		return
	}
	m.writeOAuthTokenError(c, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
		Status:  http.StatusBadRequest,
		Code:    oidErrorUnsupportedGrant,
		Message: "Unsupported grant type",
	})))
	return
}

func (m *router) userinfo(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	claims := gate.GetCtxClaims(c)
	if claims == nil {
		c.WriteError(governor.ErrWithMsg(nil, "No access token claims"))
		return
	}
	res, err := m.s.Userinfo(claims.Subject, claims.Scope)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetConnectionGroup struct {
		Userid string `valid:"userid,has" json:"-"`
		Amount int    `valid:"amount" json:"-"`
		Offset int    `valid:"offset" json:"-"`
	}
)

func (m *router) getConnections(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetConnectionGroup{
		Userid: gate.GetCtxUserid(c),
		Amount: c.QueryInt("amount", -1),
		Offset: c.QueryInt("offset", -1),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetConnections(req.Userid, req.Amount, req.Offset)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

type (
	reqGetConnection struct {
		Userid   string `valid:"userid,has" json:"-"`
		ClientID string `valid:"clientID,has" json:"-"`
	}
)

func (m *router) getConnection(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetConnection{
		Userid:   gate.GetCtxUserid(c),
		ClientID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	res, err := m.s.GetConnection(req.Userid, req.ClientID)
	if err != nil {
		c.WriteError(err)
		return
	}
	c.WriteJSON(http.StatusOK, res)
}

func (m *router) delConnection(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqGetConnection{
		Userid:   gate.GetCtxUserid(c),
		ClientID: c.Param("id"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	if err := m.s.DelConnection(req.Userid, req.ClientID); err != nil {
		c.WriteError(err)
		return
	}
	c.WriteStatus(http.StatusNoContent)
}

func (m *router) mountOidRoutes(r governor.Router) {
	scopeAuthorize := m.s.scopens + ".authorize"
	scopeConnectionRead := m.s.scopens + ".connection:read"
	scopeConnectionWrite := m.s.scopens + ".connection:write"
	r.Get("/openid-configuration", m.getOpenidConfig, m.rt)
	r.Get(jwksRoute, m.getJWKS, m.rt)
	r.Post("/auth/code", m.authCode, gate.User(m.s.gate, scopeAuthorize), m.rt)
	r.Post(tokenRoute, m.authToken, cachecontrol.ControlNoStore(m.s.logger), m.rt)
	r.Get(userinfoRoute, m.userinfo, gate.User(m.s.gate, oidScopeOpenid), m.rt)
	r.Post(userinfoRoute, m.userinfo, gate.User(m.s.gate, oidScopeOpenid), m.rt)
	r.Get("/connection", m.getConnections, gate.User(m.s.gate, scopeConnectionRead), m.rt)
	r.Get("/connection/id/{id}", m.getConnection, gate.User(m.s.gate, scopeConnectionRead), m.rt)
	r.Delete("/connection/id/{id}", m.delConnection, gate.User(m.s.gate, scopeConnectionWrite), m.rt)
}
