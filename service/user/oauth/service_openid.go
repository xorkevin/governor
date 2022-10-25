package oauth

import (
	"bytes"
	"context"
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
	"xorkevin.dev/kerrors"
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

func (s *Service) getOpenidConfig() (*resOpenidConfig, error) {
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

func (s *Service) getJWKS(ctx context.Context) (*jose.JSONWebKeySet, error) {
	return s.tokenizer.GetJWKS(ctx)
}

const (
	keySeparator  = "."
	sessionPrefix = "oauth:"
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

func (s *Service) authCode(ctx context.Context, userid, clientid, scope, nonce, challenge, method string, authTime int64) (*resAuthCode, error) {
	// sort and filter unknown scopes
	scope = dedupSSV(scope, map[string]struct{}{
		oidScopeOpenid:  {},
		oidScopeProfile: {},
		oidScopeEmail:   {},
		oidScopeOffline: {},
	})

	if _, err := s.getCachedClient(ctx, clientid); err != nil {
		if errors.Is(err, ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not found")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app")
	}

	m, err := s.connections.GetByID(ctx, userid, clientid)
	if err != nil {
		if !errors.Is(err, db.ErrorNotFound) {
			return nil, kerrors.WithMsg(err, "Failed to get oauth app connection")
		}
		m, code, err := s.connections.New(userid, clientid, scope, nonce, challenge, method, authTime)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to create oauth app connection")
		}
		if err := s.connections.Insert(ctx, m); err != nil {
			if errors.Is(err, db.ErrorUnique) {
				return nil, governor.ErrWithRes(err, http.StatusBadRequest, "", "OAuth app already connected")
			}
			return nil, kerrors.WithMsg(err, "Failed to connect oauth app")
		}
		return &resAuthCode{
			Code: userid + keySeparator + code,
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
		return nil, kerrors.WithMsg(err, "Failed to generate auth code")
	}
	if err := s.connections.Update(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to update oauth app connection")
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

	// UserinfoClaims are the claims returned for userinfo
	UserinfoClaims struct {
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
		UserinfoClaims
	}

	resUserinfo struct {
		Sub string `json:"sub"`
		UserinfoClaims
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

func (s *Service) getUserinfoClaims(ctx context.Context, userid string, scopes map[string]struct{}) (*UserinfoClaims, error) {
	claims := &UserinfoClaims{}
	user, err := s.users.GetByID(ctx, userid)
	if err != nil {
		return nil, kerrors.WithMsg(err, "User not found")
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
		bprofile := bytes.Buffer{}
		if err := s.tplprofile.Execute(&bprofile, data); err != nil {
			return nil, kerrors.WithMsg(err, "Failed executing profile url template")
		}
		bpicture := bytes.Buffer{}
		if err := s.tplpicture.Execute(&bpicture, data); err != nil {
			return nil, kerrors.WithMsg(err, "Failed executing profile picture url template")
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

func (s *Service) checkClientKey(ctx context.Context, clientid, key, redirect string) error {
	m, err := s.getCachedClient(ctx, clientid)
	if err != nil {
		if errors.Is(err, ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusUnauthorized, oidErrorInvalidClient, "Invalid client")
		}
		return kerrors.WithMsg(err, "Failed to get oauth app")
	}
	if ok, err := s.apps.ValidateKey(key, m); err != nil {
		return kerrors.WithMsg(err, "Failed to validate key")
	} else if !ok {
		return governor.ErrWithRes(nil, http.StatusUnauthorized, oidErrorInvalidClient, "Invalid client")
	}
	if redirect != m.RedirectURI {
		return governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidGrant, "Invalid redirect")
	}
	return nil
}

func (s *Service) authTokenCode(ctx context.Context, clientid, secret, redirect, userid, code, verifier string) (*resAuthToken, error) {
	if err := s.checkClientKey(ctx, clientid, secret, redirect); err != nil {
		return nil, err
	}
	m, err := s.connections.GetByID(ctx, userid, clientid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusBadRequest, oidErrorInvalidGrant, "Invalid code")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app connection")
	}
	if ok, err := s.connections.ValidateCode(code, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to validate code")
	} else if !ok {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidGrant, "Invalid code")
	}
	switch m.ChallengeMethod {
	case "":
		if verifier != "" {
			return nil, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidGrant, "Invalid code verifier")
		}
	case oidChallengePlain:
		if verifier != m.Challenge {
			return nil, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidGrant, "Invalid code verifier")
		}
	case oidChallengeS256:
		h := sha256.Sum256([]byte(verifier))
		if base64.RawURLEncoding.EncodeToString(h[:]) != m.Challenge {
			return nil, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidGrant, "Invalid code verifier")
		}
	default:
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "Invalid code challenge method")
	}

	now := time.Now().Round(0)
	if now.After(time.Unix(m.CodeTime, 0).Add(s.codeDuration)) {
		return nil, governor.ErrWithRes(nil, http.StatusBadRequest, oidErrorInvalidRequest, "Code expired")
	}

	scopes := ssvSet(m.Scope)

	m.CodeHash = ""
	m.CodeTime = 0
	m.AccessTime = now.Unix()

	var key string
	if _, ok := scopes[oidScopeOffline]; ok {
		key, err = s.connections.RehashKey(m)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to generate oauth session key")
		}
	} else {
		m.KeyHash = ""
	}

	sessionID := sessionPrefix + userid + keySeparator + clientid
	accessToken, _, err := s.tokenizer.Generate(ctx, token.KindAccess, userid, s.accessDuration, sessionID, m.AuthTime, m.Scope, "")
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate access token")
	}
	var refreshToken string
	if key != "" {
		refreshToken, _, err = s.tokenizer.Generate(ctx, token.KindOAuthRefresh, userid, s.refreshDuration, sessionID, m.AuthTime, m.Scope, key)
		if err != nil {
			return nil, kerrors.WithMsg(err, "Failed to generate refresh token")
		}
	}

	userClaims, err := s.getUserinfoClaims(ctx, userid, scopes)
	if err != nil {
		return nil, err
	}

	claims := idTokenClaims{
		Nonce:          m.Nonce,
		Azp:            clientid,
		UserinfoClaims: *userClaims,
	}
	idToken, err := s.tokenizer.GenerateExt(ctx, token.KindOAuthID, s.issuer, userid, []string{clientid}, s.accessDuration, sessionID, m.AuthTime, claims)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to generate id token")
	}

	if err := s.connections.Update(ctx, m); err != nil {
		return nil, kerrors.WithMsg(err, "Failed to update oauth session")
	}

	return &resAuthToken{
		AccessToken:  accessToken,
		TokenType:    oidTokenTypeBearer,
		ExpiresIn:    int64(s.accessDuration / time.Second),
		RefreshToken: refreshToken,
		Scope:        m.Scope,
		IDToken:      idToken,
	}, nil
}

func (s *Service) userinfo(ctx context.Context, userid string, scope string) (*resUserinfo, error) {
	userClaims, err := s.getUserinfoClaims(ctx, userid, ssvSet(scope))
	if err != nil {
		return nil, err
	}

	return &resUserinfo{
		Sub:            userid,
		UserinfoClaims: *userClaims,
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

func (s *Service) getConnections(ctx context.Context, userid string, amount, offset int) (*resConnections, error) {
	m, err := s.connections.GetUserConnections(ctx, userid, amount, offset)
	if err != nil {
		return nil, kerrors.WithMsg(err, "Failed to get oauth app connections")
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

func (s *Service) getConnection(ctx context.Context, userid string, clientid string) (*resConnection, error) {
	m, err := s.connections.GetByID(ctx, userid, clientid)
	if err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return nil, governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not connected")
		}
		return nil, kerrors.WithMsg(err, "Failed to get oauth app connection")
	}
	return &resConnection{
		ClientID:     m.ClientID,
		Scope:        m.Scope,
		AuthTime:     m.AuthTime,
		AccessTime:   m.AccessTime,
		CreationTime: m.CreationTime,
	}, nil
}

func (s *Service) delConnection(ctx context.Context, userid string, clientid string) error {
	if _, err := s.connections.GetByID(ctx, userid, clientid); err != nil {
		if errors.Is(err, db.ErrorNotFound) {
			return governor.ErrWithRes(err, http.StatusNotFound, "", "OAuth app not connected")
		}
		return kerrors.WithMsg(err, "Failed to get oauth app connection")
	}
	if err := s.connections.Delete(ctx, userid, []string{clientid}); err != nil {
		return kerrors.WithMsg(err, "Failed to delete oauth app connection")
	}
	return nil
}

func (s *Service) deleteUserConnections(ctx context.Context, userid string) error {
	if err := s.connections.DeleteUserConnections(ctx, userid); err != nil {
		return kerrors.WithMsg(err, "Failed to delete user oauth app connections")
	}
	return nil
}
