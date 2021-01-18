package oauth

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_openid_gen.go reqOAuthAuthorize reqGetConnectionGroup reqGetConnection

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
		c.WriteError(governor.NewCodeError(oidErrorInvalidRequest, "No code challenge method provided", http.StatusBadRequest, nil))
		return
	}
	if req.CodeChallengeMethod != "" && req.CodeChallenge == "" {
		c.WriteError(governor.NewCodeError(oidErrorInvalidRequest, "No code challenge provided", http.StatusBadRequest, nil))
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

const (
	scopeAuthorize       = "gov.user.oauth.authorize"
	scopeConnectionRead  = "gov.user.oauth.connections:read"
	scopeConnectionWrite = "gov.user.oauth.connections:write"
)

func (m *router) mountOidRoutes(r governor.Router) {
	r.Get("/openid-configuration", m.getOpenidConfig)
	r.Get(jwksRoute, m.getJWKS)
	r.Post("/auth/code", m.authCode, gate.User(m.s.gate, scopeAuthorize))
	r.Get("/connection", m.getConnections, gate.User(m.s.gate, scopeConnectionRead))
	r.Get("/connection/id/{id}", m.getConnection, gate.User(m.s.gate, scopeConnectionRead))
}
