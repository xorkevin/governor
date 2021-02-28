package oauth

import (
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
		c.WriteError(governor.NewCodeErrorUser(oidErrorInvalidRequest, "No code challenge method provided", http.StatusBadRequest, nil))
		return
	}
	if req.CodeChallengeMethod != "" && req.CodeChallenge == "" {
		c.WriteError(governor.NewCodeErrorUser(oidErrorInvalidRequest, "No code challenge provided", http.StatusBadRequest, nil))
		return
	}

	res, err := m.s.AuthCode(req.Userid, req.ClientID, req.Scope, req.Nonce, req.CodeChallenge, req.CodeChallengeMethod)
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
	if governor.ErrorStatus(err) == http.StatusUnauthorized {
		c.SetHeader("WWW-Authenticate", "Basic realm=\"governor\"")
	}
	c.WriteJSON(governor.ErrorStatus(err), resAuthTokenErr{
		Error: governor.ErrorCode(err),
		Desc:  governor.ErrorMsg(err),
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
				m.writeOAuthTokenError(c, governor.NewCodeErrorUser(oidErrorInvalidRequest, "Client secret basic and post used", http.StatusBadRequest, nil))
				return
			}
			var err error
			req.ClientID, err = url.QueryUnescape(user)
			if err != nil {
				m.writeOAuthTokenError(c, governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid client id encoding", http.StatusBadRequest, err))
				return
			}
			req.ClientSecret, err = url.QueryUnescape(pass)
			if err != nil {
				m.writeOAuthTokenError(c, governor.NewCodeErrorUser(oidErrorInvalidRequest, "Invalid client secret encoding", http.StatusBadRequest, err))
				return
			}
		}
		if j := strings.SplitN(c.FormValue("code"), "|", 2); len(j) == 2 {
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
	m.writeOAuthTokenError(c, governor.NewCodeErrorUser(oidErrorUnsupportedGrant, "Unsupported grant type", http.StatusBadRequest, nil))
	return
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

const (
	scopeAuthorize       = "gov.user.oauth.authorize"
	scopeConnectionRead  = "gov.user.oauth.connection:read"
	scopeConnectionWrite = "gov.user.oauth.connection:write"
)

func (m *router) mountOidRoutes(r governor.Router) {
	r.Get("/openid-configuration", m.getOpenidConfig)
	r.Get(jwksRoute, m.getJWKS)
	r.Post("/auth/code", m.authCode, gate.User(m.s.gate, scopeAuthorize))
	r.Post(tokenRoute, m.authToken, cachecontrol.NoStore(m.s.logger))
	r.Get("/connection", m.getConnections, gate.User(m.s.gate, scopeConnectionRead))
	r.Get("/connection/id/{id}", m.getConnection, gate.User(m.s.gate, scopeConnectionRead))
	r.Delete("/connection/id/{id}", m.delConnection, gate.User(m.s.gate, scopeConnectionWrite))
}
