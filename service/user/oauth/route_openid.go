package oauth

import (
	"net/http"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_openid_gen.go reqOidAuthorize

type (
	reqOidAuthorize struct {
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

func (m *router) oidAuthorize(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOidAuthorize{}
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

const (
	scopeOAuthAuthorize = "gov.user.oauth.authorize"
)

func (m *router) mountOidRoutes(r governor.Router) {
	r.Get("/openid-configuration", m.getOpenidConfig)
	r.Get(jwksRoute, m.getJWKS)
	r.Post("/auth/code", m.oidAuthorize, gate.User(m.s.gate, scopeOAuthAuthorize))
}
