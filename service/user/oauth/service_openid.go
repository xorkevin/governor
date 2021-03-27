package oauth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"gopkg.in/square/go-jose.v2"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/user/token"
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

	oidTokenTypeBearer = "Bearer"

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
			"azp",
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

const (
	keySeparator = "|"
)

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

func (s *service) AuthCode(userid, clientid, scope, nonce, challenge, method string, authTime int64) (*resAuthCode, error) {
	// sort and filter unknown scopes
	scope = dedupSSV(scope, map[string]struct{}{
		oidScopeOpenid:  {},
		oidScopeProfile: {},
		oidScopeEmail:   {},
		oidScopeOffline: {},
	})

	if _, err := s.getCachedClient(clientid); err != nil {
		if errors.Is(err, ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not found",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app")
	}

	m, err := s.connections.GetByID(userid, clientid)
	if err != nil {
		if !errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.ErrWithMsg(err, "Failed to get oauth app connection")
		}
		m, code, err := s.connections.New(userid, clientid, scope, nonce, challenge, method, authTime)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to create oauth app connection")
		}
		if err := s.connections.Insert(m); err != nil {
			if errors.Is(err, db.ErrUnique{}) {
				return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
					Status:  http.StatusBadRequest,
					Message: "OAuth app already connected",
				}), governor.ErrOptInner(err))
			}
			return nil, governor.ErrWithMsg(err, "Failed to connect oauth app")
		}
		return &resAuthCode{
			Code: code,
		}, nil
	}

	now := time.Now().Round(0).Unix()

	m.Scope = scope
	m.Nonce = nonce
	m.Challenge = challenge
	m.ChallengeMethod = method
	m.AuthTime = authTime
	m.CodeTime = now
	m.AccessTime = now
	code, err := s.connections.RehashCode(m)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to generate auth code")
	}
	if err := s.connections.Update(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update oauth app connection")
	}
	return &resAuthCode{
		Code: userid + keySeparator + code,
	}, nil
}

type (
	resAuthToken struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int64  `json:"expires_in"`
		RefreshToken string `json:"refresh_token,omitempty"`
		Scope        string `json:"scope"`
		IDToken      string `json:"id_token"`
	}

	userinfoClaims struct {
		Name              string `json:"name,omitempty"`
		FamilyName        string `json:"family_name,omitempty"`
		GivenName         string `json:"given_name,omitempty"`
		PreferredUsername string `json:"preferred_username,omitempty"`
		Profile           string `json:"profile,omitempty"`
		Picture           string `json:"picture,omitempty"`
		Email             string `json:"email,omitempty"`
		EmailVerified     bool   `json:"email_verified,omitempty"`
	}

	idTokenClaims struct {
		Nonce string `json:"nonce,omitempty"`
		Azp   string `json:"azp,omitempty"`
		userinfoClaims
	}

	profileURLData struct {
		Userid   string
		Username string
	}
)

func ssvSet(s string) map[string]struct{} {
	k := strings.Fields(s)
	scopes := make(map[string]struct{}, len(k))
	for _, i := range k {
		scopes[i] = struct{}{}
	}
	return scopes
}

func (s *service) Userinfo(userid string, scope string) (*userinfoClaims, error) {
	scopes := ssvSet(scope)

	claims := &userinfoClaims{}
	user, err := s.users.GetByID(userid)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "User not found")
	}
	if _, ok := scopes[oidScopeProfile]; ok {
		claims.Name = fmt.Sprintf("%s %s", user.FirstName, user.LastName)
		claims.FamilyName = user.LastName
		claims.GivenName = user.FirstName
		claims.PreferredUsername = user.Username
		data := profileURLData{
			Userid:   user.Userid,
			Username: user.Username,
		}
		bprofile := &bytes.Buffer{}
		if err := s.tplprofile.Execute(bprofile, data); err != nil {
			return nil, governor.ErrWithMsg(err, "Failed executing profile url template")
		}
		bpicture := &bytes.Buffer{}
		if err := s.tplpicture.Execute(bpicture, data); err != nil {
			return nil, governor.ErrWithMsg(err, "Failed executing profile picture url template")
		}
		claims.Profile = bprofile.String()
		claims.Picture = bpicture.String()
	}
	if _, ok := scopes[oidScopeEmail]; ok {
		claims.Email = user.Email
		claims.EmailVerified = true
	}
	return claims, nil
}

func (s *service) checkClientKey(clientid, key, redirect string) error {
	m, err := s.getCachedClient(clientid)
	if err != nil {
		if errors.Is(err, ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusUnauthorized,
				Code:    oidErrorInvalidClient,
				Message: "Invalid client",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get oauth app")
	}
	if ok, err := s.apps.ValidateKey(key, m); err != nil {
		return governor.ErrWithMsg(err, "Failed to validate key")
	} else if !ok {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusUnauthorized,
			Code:    oidErrorInvalidClient,
			Message: "Invalid client",
		}))
	}
	if redirect != m.RedirectURI {
		return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidGrant,
			Message: "Invalid redirect",
		}))
	}
	return nil
}

func (s *service) AuthTokenCode(clientid, secret, redirect, userid, code, verifier string) (*resAuthToken, error) {
	if err := s.checkClientKey(clientid, secret, redirect); err != nil {
		return nil, err
	}
	m, err := s.connections.GetByID(userid, clientid)
	if err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Code:    oidErrorInvalidGrant,
				Message: "Invalid code",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app connection")
	}
	if ok, err := s.connections.ValidateCode(code, m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to validate code")
	} else if !ok {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidGrant,
			Message: "Invalid code",
		}))
	}
	switch m.ChallengeMethod {
	case "":
		if verifier != "" {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Code:    oidErrorInvalidGrant,
				Message: "Invalid code verifier",
			}))
		}
	case oidChallengePlain:
		if verifier != m.Challenge {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Code:    oidErrorInvalidGrant,
				Message: "Invalid code verifier",
			}))
		}
	case oidChallengeS256:
		h := sha256.Sum256([]byte(verifier))
		if base64.RawURLEncoding.EncodeToString(h[:]) != m.Challenge {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusBadRequest,
				Code:    oidErrorInvalidGrant,
				Message: "Invalid code verifier",
			}))
		}
	default:
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Invalid code challenge method",
		}))
	}

	now := time.Now().Round(0).Unix()
	if now > m.CodeTime+s.codeTime {
		return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
			Status:  http.StatusBadRequest,
			Code:    oidErrorInvalidRequest,
			Message: "Code expired",
		}))
	}

	scopes := ssvSet(m.Scope)

	m.CodeHash = ""
	m.CodeTime = 0
	m.AccessTime = now

	var key string
	if _, ok := scopes[oidScopeOffline]; ok {
		key, err = s.connections.RehashKey(m)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to generate oauth session key")
		}
	} else {
		m.KeyHash = ""
	}

	if err := s.connections.Update(m); err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to update oauth session")
	}

	sessionID := "oauth:" + userid + keySeparator + clientid
	accessToken, _, err := s.tokenizer.Generate(token.KindAccess, userid, s.accessTime, sessionID, m.AuthTime, m.Scope, "")
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to generate access token")
	}
	var refreshToken string
	if key != "" {
		refreshToken, _, err = s.tokenizer.Generate(token.KindOAuthRefresh, userid, s.refreshTime, sessionID, m.AuthTime, m.Scope, key)
		if err != nil {
			return nil, governor.ErrWithMsg(err, "Failed to generate refresh token")
		}
	}

	userClaims, err := s.Userinfo(userid, m.Scope)
	if err != nil {
		return nil, err
	}

	claims := idTokenClaims{
		Nonce:          m.Nonce,
		Azp:            clientid,
		userinfoClaims: *userClaims,
	}
	idToken, err := s.tokenizer.GenerateExt(token.KindOAuthID, s.issuer, userid, []string{clientid}, s.accessTime, sessionID, m.AuthTime, claims)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to generate id token")
	}

	return &resAuthToken{
		AccessToken:  accessToken,
		TokenType:    oidTokenTypeBearer,
		ExpiresIn:    s.accessTime,
		RefreshToken: refreshToken,
		Scope:        m.Scope,
		IDToken:      idToken,
	}, nil
}

type (
	resConnection struct {
		ClientID     string `json:"client_id"`
		Scope        string `json:"scope"`
		AuthTime     int64  `json:"auth_time"`
		AccessTime   int64  `json:"access_time"`
		CreationTime int64  `json:"creation_time"`
	}

	resConnections struct {
		Connections []resConnection `json:"connections"`
	}
)

func (s *service) GetConnections(userid string, amount, offset int) (*resConnections, error) {
	m, err := s.connections.GetUserConnections(userid, amount, offset)
	if err != nil {
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app connections")
	}
	res := make([]resConnection, 0, len(m))
	for _, i := range m {
		res = append(res, resConnection{
			ClientID:     i.ClientID,
			Scope:        i.Scope,
			AuthTime:     i.AuthTime,
			AccessTime:   i.AccessTime,
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
		if errors.Is(err, db.ErrNotFound{}) {
			return nil, governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not connected",
			}), governor.ErrOptInner(err))
		}
		return nil, governor.ErrWithMsg(err, "Failed to get oauth app connection")
	}
	return &resConnection{
		ClientID:     m.ClientID,
		Scope:        m.Scope,
		AuthTime:     m.AuthTime,
		AccessTime:   m.AccessTime,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *service) DelConnection(userid string, clientid string) error {
	if _, err := s.connections.GetByID(userid, clientid); err != nil {
		if errors.Is(err, db.ErrNotFound{}) {
			return governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
				Status:  http.StatusNotFound,
				Message: "OAuth app not connected",
			}), governor.ErrOptInner(err))
		}
		return governor.ErrWithMsg(err, "Failed to get oauth app connection")
	}
	if err := s.connections.Delete(userid, []string{clientid}); err != nil {
		return governor.ErrWithMsg(err, "Failed to delete oauth app connection")
	}
	return nil
}
