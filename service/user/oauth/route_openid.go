package oauth

import (
	"net/http"
	"xorkevin.dev/governor"
)

//go:generate forge validation -o validation_openid_gen.go reqOidAuthorize

type (
	reqOidAuthorize struct {
		ResponseType        string `valid:"oidResponseType" json:"-"`
		ResponseMode        string `valid:"oidResponseMode" json:"-"`
		ClientID            string `valid:"clientID,has" json:"-"`
		Scope               string `valid:"oidScope" json:"-"`
		RedirectURI         string `valid:"redirect" json:"-"`
		State               string `valid:"oidState" json:"-"`
		Nonce               string `valid:"oidNonce" json:"-"`
		CodeChallenge       string `valid:"oidCodeChallenge" json:"-"`
		CodeChallengeMethod string `valid:"oidCodeChallengeMethod" json:"-"`
		Display             string `valid:"oidDisplay" json:"-"`
		Prompt              string `valid:"oidPrompt" json:"-"`
		MaxAge              int    `valid:"oidMaxAge"`
		IDTokenHint         string `json:"-"`
		LoginHint           string `valid:"oidLoginHint" json:"-"`
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
	c.WriteStatus(http.StatusOK)
}

func (m *router) mountOidRoutes(r governor.Router) {
	r.Get("/openid-configuration", m.getOpenidConfig)
	r.Get("/jwks", m.getJWKS)
	ar := r.Group("/auth/authorize")
	ar.Get("", m.oidAuthorize)
	ar.Post("", m.oidAuthorize)
}
