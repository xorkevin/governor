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

	oidGrantTypeCode    = "authorization_code"
	oidGrantTypeRefresh = "refresh_token"

	oidErrorInvalidRequest   = "invalid_request"
	oidErrorInvalidScope     = "invalid_scope"
	oidErrorInvalidClient    = "invalid_client"
	oidErrorInvalidGrant     = "invalid_grant"
	oidErrorUnsupportedGrant = "unsupported_grant_type"
	oidErrorServer           = "server_error"
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
		GrantTypes:   []string{oidGrantTypeCode, oidGrantTypeRefresh},
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
		Code: userid + "|" + code,
	}, nil
}

type (
	resAuthToken struct {
		AccessToken string `json:"access_token"`
	}
)

func (s *service) checkClientKey(clientid, key, redirect string) error {
	m, err := s.getCachedClient(clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewCodeErrorUser(oidErrorInvalidClient, "Invalid client", http.StatusUnauthorized, nil)
		}
		return governor.NewCodeError(oidErrorServer, "", http.StatusInternalServerError, err)
	}
	if ok, err := s.apps.ValidateKey(key, m); err != nil || !ok {
		return governor.NewCodeErrorUser(oidErrorInvalidClient, "Invalid client", http.StatusUnauthorized, nil)
	}
	if redirect != m.RedirectURI {
		return governor.NewCodeErrorUser(oidErrorInvalidGrant, "Invalid redirect", http.StatusBadRequest, nil)
	}
	return nil
}

func (s *service) AuthTokenCode(clientid, secret, userid, code, verifier, redirect string) (*resAuthToken, error) {
	if err := s.checkClientKey(clientid, secret, redirect); err != nil {
		return nil, err
	}
	m, err := s.connections.GetByID(userid, clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewCodeErrorUser(oidErrorInvalidGrant, "Invalid code", http.StatusBadRequest, nil)
		}
		return nil, governor.NewCodeError(oidErrorServer, "", http.StatusInternalServerError, err)
	}
	if ok, err := s.connections.ValidateCode(code, m); err != nil || !ok {
		return nil, governor.NewCodeError(oidErrorInvalidGrant, "Invalid code", http.StatusBadRequest, nil)
	}
	return nil, nil
}

type (
	resConnection struct {
		ClientID     string `json:"client_id"`
		Scope        string `json:"scope"`
		Time         int64  `json:"time"`
		CreationTime int64  `json:"creation_time"`
	}

	resConnections struct {
		Connections []resConnection `json:"connections"`
	}
)

func (s *service) GetConnections(userid string, amount, offset int) (*resConnections, error) {
	m, err := s.connections.GetUserConnections(userid, amount, offset)
	if err != nil {
		return nil, err
	}
	res := make([]resConnection, 0, len(m))
	for _, i := range m {
		res = append(res, resConnection{
			ClientID:     i.ClientID,
			Scope:        i.Scope,
			Time:         i.Time,
			CreationTime: i.CreationTime,
		})
	}
	return &resConnections{
		Connections: res,
	}, nil
}

func (s *service) GetConnection(userid string, clientid string) (*resConnection, error) {
	m, err := s.connections.GetByID(userid, clientid)
	if err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return nil, governor.NewErrorUser("", 0, err)
		}
		return nil, err
	}
	return &resConnection{
		ClientID:     m.ClientID,
		Scope:        m.Scope,
		Time:         m.Time,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) DelConnection(userid string, clientid string) error {
	if _, err := s.connections.GetByID(userid, clientid); err != nil {
		if governor.ErrorStatus(err) == http.StatusNotFound {
			return governor.NewErrorUser("", 0, err)
		}
		return err
	}
	if err := s.connections.Delete(userid, []string{clientid}); err != nil {
		return err
	}
	return nil
}
