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
		ResponseType        string `valid:"oidResponseType" json:"response_type"`
		ResponseMode        string `valid:"oidResponseMode" json:"response_mode"`
		ClientID            string `valid:"clientID,has" json:"client_id"`
		Scope               string `valid:"oidScope" json:"scope"`
		RedirectURI         string `valid:"redirect" json:"redirect_uri"`
		State               string `valid:"oidState" json:"state"`
		Nonce               string `valid:"oidNonce" json:"nonce"`
		CodeChallenge       string `valid:"oidCodeChallenge" json:"code_challenge"`
		CodeChallengeMethod string `valid:"oidCodeChallengeMethod" json:"code_challenge_method"`
		Display             string `valid:"oidDisplay" json:"display"`
		Prompt              string `valid:"oidPrompt" json:"prompt"`
		MaxAge              int    `valid:"oidMaxAge" json:"max_age"`
		IDTokenHint         string `valid:"oidIDTokenHint" json:"id_token_hint"`
		LoginHint           string `valid:"oidLoginHint" json:"login_hint"`
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
		ResponseType:        c.Query("response_type"),
		ResponseMode:        c.QueryDef("response_mode", oidResponseModeQuery),
		ClientID:            c.Query("client_id"),
		Scope:               c.Query("scope"),
		RedirectURI:         c.Query("redirect_uri"),
		State:               c.Query("state"),
		Nonce:               c.Query("nonce"),
		CodeChallenge:       c.Query("code_challenge"),
		CodeChallengeMethod: c.QueryDef("code_challenge_method", oidChallengePlain),
		Display:             c.QueryDef("display", oidDisplayPage),
		Prompt:              c.Query("prompt"),
		MaxAge:              c.QueryInt("max_age", -1),
		IDTokenHint:         c.Query("id_token_hint"),
		LoginHint:           c.Query("login_hint"),
	}
	if err := req.valid(); err != nil {
		c.WriteError(err)
		return
	}

	// check if client app exists and redirect uri matches
	if m, err := m.s.GetApp(req.ClientID); err != nil {
		c.WriteError(err)
		return
	} else if m.RedirectURI != req.RedirectURI {
		c.WriteError(governor.NewErrorUser("Redirect URI does not match", http.StatusBadRequest, nil))
		return
	}
	// filter unknown scopes
	req.Scope = dedupSSV(req.Scope, map[string]struct{}{
		oidScopeOpenid:  {},
		oidScopeProfile: {},
		oidScopeEmail:   {},
		oidScopeOffline: {},
	})
	// filter prompts
	req.Prompt = dedupSSV(req.Prompt, map[string]struct{}{
		oidPromptNone:       {},
		oidPromptLogin:      {},
		oidPromptConsent:    {},
		oidPromptSelectAcct: {},
	})

	if req.DoneLogin == 0 && req.MaxAge > -1 {
		// validate user session
	}

	c.WriteStatus(http.StatusOK)
}

func (m *router) mountOidRoutes(r governor.Router) {
	r.Get("/openid-configuration", m.getOpenidConfig)
	r.Get("/jwks", m.getJWKS)
	ar := r.Group("/auth/authorize")
	ar.Get("", m.oidAuthorize, gate.TryUser(m.s.gate, ""))
	ar.Post("", m.oidAuthorize, gate.TryUser(m.s.gate, ""))
}
