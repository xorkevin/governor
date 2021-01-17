package oauth

import (
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/gate"
)

//go:generate forge validation -o validation_openid_gen.go reqOidAuthorize

type (
	reqOidAuthorize struct {
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

func dedupSSV(s string, allowed map[string]struct{}) string {
	k := strings.Fields(s)
	next := make([]string, 0, len(k))
	nextSet := make(map[string]struct{}, len(k))
	for _, i := range k {
		if _, ok := allowed[i]; ok {
			if _, ok := nextSet[i]; !ok {
				nextSet[i] = struct{}{}
				next = append(next, i)
			}
		}
	}
	return strings.Join(next, " ")
}

func (m *router) oidAuthorize(w http.ResponseWriter, r *http.Request) {
	c := governor.NewContext(w, r, m.s.logger)
	req := reqOidAuthorize{
		ClientID:            c.Query("client_id"),
		Scope:               c.Query("scope"),
		Nonce:               c.Query("nonce"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.QueryDef("code_challenge_method", oidChallengePlain),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	// filter unknown scopes
	req.Scope = dedupSSV(req.Scope, map[string]struct{}{
		oidScopeOpenid:  {},
		oidScopeProfile: {},
		oidScopeEmail:   {},
		oidScopeOffline: {},
	})

	c.WriteStatus(http.StatusOK)
}

const (
	scopeOAuthAuthorize = "gov.user.oauth.authorize"
)

func (m *router) mountOidRoutes(r governor.Router) {
	r.Get("/openid-configuration", m.getOpenidConfig)
	r.Get(jwksRoute, m.getJWKS)
	r.Post("/auth/code", m.oidAuthorize, gate.User(m.s.gate, scopeOAuthAuthorize))
}
