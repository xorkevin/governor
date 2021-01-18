package oauth

import (
	"gopkg.in/square/go-jose.v2"
	"net/http"
	"sort"
	"strings"
	"xorkevin.dev/governor"
)

const (
	oidResponseTypeCode = "code"

	oidResponseModeQuery    = "query"
	oidResponseModeFragment = "fragment"

	oidScopeOpenid  = "openid"
	oidScopeProfile = "profile"
	oidScopeEmail   = "email"
	oidScopeOffline = "offline_access"

	oidChallengePlain = "plain"
	oidChallengeS256  = "S256"

	oidErrorInvalidRequest = "invalid_request"
	oidErrorInvalidScope   = "invalid_scope"
)

type (
	resOpenidConfig struct {
		Issuer          string   `json:"issuer"`
		EPAuthorization string   `json:"authorization_endpoint"`
		EPToken         string   `json:"token_endpoint"`
		EPUserinfo      string   `json:"userinfo_endpoint"`
		EPJwks          string   `json:"jwks_uri"`
		Scopes          []string `json:"scopes_supported"`
		ResponseTypes   []string `json:"response_types_supported"`
		ResponseModes   []string `json:"response_modes_supported"`
		GrantTypes      []string `json:"grant_types_supported"`
		SubjectTypes    []string `json:"subject_types_supported"`
		SigningAlgs     []string `json:"id_token_signing_alg_values_supported"`
		EPTokenAuth     []string `json:"token_endpoint_auth_methods_supported"`
		CodeChallenge   []string `json:"code_challenge_methods_supported"`
		Claims          []string `json:"claims_supported"`
	}
)

func (s *service) GetOpenidConfig() (*resOpenidConfig, error) {
	return &resOpenidConfig{
		Issuer:          s.issuer,
		EPAuthorization: s.epauth,
		EPToken:         s.eptoken,
		EPUserinfo:      s.epuserinfo,
		EPJwks:          s.epjwks,
		Scopes: []string{
			oidScopeOpenid,
			oidScopeProfile,
			oidScopeEmail,
			oidScopeOffline,
		},
		ResponseTypes: []string{oidResponseTypeCode},
		ResponseModes: []string{
			oidResponseModeQuery,
			oidResponseModeFragment,
		},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		SubjectTypes: []string{"public"},
		SigningAlgs:  []string{"RS256"},
		EPTokenAuth:  []string{"client_secret_basic", "client_secret_post"},
		CodeChallenge: []string{
			oidChallengePlain,
			oidChallengeS256,
		},
		Claims: []string{
			"iss",
			"sub",
			"aud",
			"iat",
			"nbf",
			"exp",
			"auth_time",
			"nonce",
			"name",
			"given_name",
			"family_name",
			"preferred_username",
			"profile",
			"picture",
			"email",
			"email_verified",
		},
	}, nil
}

func (s *service) GetJWKS() (*jose.JSONWebKeySet, error) {
	return s.tokenizer.GetJWKS(), nil
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
	sort.Strings(next)
	return strings.Join(next, " ")
}

type (
	resAuthCode struct {
		Code string `json:"code"`
	}
)

func (s *service) AuthCode(userid, clientid, scope, nonce, challenge, method string) (*resAuthCode, error) {
	// sort and filter unknown scopes
	scope = dedupSSV(scope, map[string]struct{}{
		oidScopeOpenid:  {},
		oidScopeProfile: {},
		oidScopeEmail:   {},
		oidScopeOffline: {},
	})

	m, err := s.connections.GetByID(userid, clientid)
	if err != nil {
		if governor.ErrorStatus(err) != http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		m, code, err := s.connections.New(userid, clientid, scope, nonce, challenge, method)
		if err != nil {
			return nil, err
		}
		if err := s.connections.Insert(m); err != nil {
			if governor.ErrorStatus(err) == http.StatusBadRequest {
				return nil, governor.NewErrorUser("", 0, err)
			}
			return nil, err
		}
		return &resAuthCode{
			Code: code,
		}, nil
	}

	m.Scope = scope
	m.Nonce = nonce
	m.Challenge = challenge
	m.ChallengeMethod = method
	code, err := s.connections.RehashCode(m)
	if err != nil {
		return nil, err
	}
	if err := s.connections.Update(m); err != nil {
		return nil, err
	}
	return &resAuthCode{
		Code: code,
	}, nil
}
