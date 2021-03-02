package token

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"net/http"
	"strings"
	"time"

	_ "golang.org/x/crypto/blake2b" // depends on registering blake2b hash
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"xorkevin.dev/governor"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "all"
)

const (
	// KindAccess is an access token kind
	KindAccess = "access"
	// KindRefresh is a refresh token kind
	KindRefresh = "refresh"
	// KindOAuthAccess is an oauth access token kind
	KindOAuthAccess = "oauth:access"
	// KindOAuthRefresh is an oauth refresh token kind
	KindOAuthRefresh = "oauth:refresh"
	// KindOAuthID is an openid id token kind
	KindOAuthID = "oauth:id"
)

const (
	pemBlockType = "PRIVATE KEY"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		jwt.Claims
		Kind     string `json:"kind"`
		AuthTime int64  `json:"auth_time,omitempty"`
		Scope    string `json:"scope,omitempty"`
		Key      string `json:"key,omitempty"`
	}

	// Tokenizer is a token generator
	Tokenizer interface {
		GetJWKS() *jose.JSONWebKeySet
		Generate(kind string, userid string, duration int64, id string, authTime int64, scope string, key string) (string, *Claims, error)
		GenerateExt(kind string, userid string, audience []string, duration int64, id string, claims interface{}) (string, error)
		Validate(kind string, tokenString string, scope string) (bool, *Claims)
		GetClaims(kind string, tokenString string, scope string) (bool, *Claims)
		GetClaimsExt(kind string, tokenString string, audience []string, scope string, claims interface{}) (bool, *Claims)
	}

	// Service is a Tokenizer and governor.Service
	Service interface {
		governor.Service
		Tokenizer
	}

	service struct {
		secret     []byte
		privateKey crypto.Signer
		publicKey  crypto.PublicKey
		issuer     string
		audience   string
		signer     jose.Signer
		keySigner  jose.Signer
		jwk        *jose.JSONWebKey
		logger     governor.Logger
	}

	ctxKeyTokenizer struct{}
)

// GetCtxTokenizer returns a Tokenizer from the context
func GetCtxTokenizer(inj governor.Injector) Tokenizer {
	v := inj.Get(ctxKeyTokenizer{})
	if v == nil {
		return nil
	}
	return v.(Tokenizer)
}

// setCtxTokenizer sets a Tokenizer in the context
func setCtxTokenizer(inj governor.Injector, t Tokenizer) {
	inj.Set(ctxKeyTokenizer{}, t)
}

// New creates a new Tokenizer
func New() Service {
	return &service{
		secret:   nil,
		issuer:   "",
		audience: "",
		signer:   nil,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxTokenizer(inj, s)

	r.SetDefault("tokensecret", "")
	r.SetDefault("issuer", "governor")
	r.SetDefault("audience", "governor")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})

	tokensecret, err := r.GetSecret("tokensecret")
	if err != nil {
		return governor.NewError("Failed to read token secret", http.StatusInternalServerError, err)
	}
	secret, ok := tokensecret["secret"].(string)
	if !ok {
		return governor.NewError("Invalid secret", http.StatusInternalServerError, nil)
	}
	if secret == "" {
		return governor.NewError("Token secret is not set", http.StatusBadRequest, nil)
	}
	s.secret = []byte(secret)
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: s.secret}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return governor.NewError("Failed to create new jwt signer", http.StatusInternalServerError, err)
	}
	s.signer = sig

	rsakeysecret, err := r.GetSecret("rsakey")
	if err != nil {
		return governor.NewError("Failed to read rsakey", http.StatusInternalServerError, err)
	}
	rsakeyPem, ok := rsakeysecret["secret"].(string)
	if !ok {
		return governor.NewError("Invalid rsakey", http.StatusInternalServerError, nil)
	}
	pemBlock, _ := pem.Decode([]byte(rsakeyPem))
	if pemBlock == nil || pemBlock.Type != pemBlockType {
		return governor.NewError("Invalid rsakey", http.StatusInternalServerError, nil)
	}
	rawKey, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)
	if err != nil {
		return governor.NewError("Invalid rsakey", http.StatusInternalServerError, err)
	}
	key, ok := rawKey.(*rsa.PrivateKey)
	if !ok {
		return governor.NewError("Invalid rsakey", http.StatusInternalServerError, nil)
	}
	key.Precompute()
	s.privateKey = key
	s.publicKey = key.Public()
	keySig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: s.privateKey}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return governor.NewError("Failed to create new jwt RS256 signer", http.StatusInternalServerError, err)
	}
	s.keySigner = keySig

	jwk := &jose.JSONWebKey{
		Key:       s.publicKey,
		Algorithm: "RS256",
		Use:       "sig",
	}
	kid, err := jwk.Thumbprint(crypto.BLAKE2b_512)
	if err != nil {
		return governor.NewError("Failed to calculate jwk thumbprint", http.StatusInternalServerError, err)
	}
	jwk.KeyID = base64.RawURLEncoding.EncodeToString(kid)
	s.jwk = jwk

	issuer := r.GetStr("issuer")
	if issuer == "" {
		return governor.NewError("Token issuer is not set", http.StatusBadRequest, nil)
	}
	s.issuer = issuer

	l.Info("loaded config", map[string]string{
		"issuer": issuer,
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) Start(ctx context.Context) error {
	return nil
}

func (s *service) Stop(ctx context.Context) {
}

func (s *service) Health() error {
	return nil
}

// GetJWKS returns an RFC 7517 representation of the public signing key
func (s *service) GetJWKS() *jose.JSONWebKeySet {
	keys := make([]jose.JSONWebKey, 0, 1)
	if s.jwk != nil {
		keys = append(keys, *s.jwk)
	}
	return &jose.JSONWebKeySet{
		Keys: keys,
	}
}

// Generate returns a new jwt token from a user model
func (s *service) Generate(kind string, userid string, duration int64, id string, authTime int64, scope string, key string) (string, *Claims, error) {
	now := time.Now().Round(0)
	claims := Claims{
		Claims: jwt.Claims{
			Issuer:    s.issuer,
			Subject:   userid,
			Audience:  []string{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
			ID:        id,
		},
		Kind:     kind,
		AuthTime: authTime,
		Scope:    scope,
		Key:      key,
	}
	token, err := jwt.Signed(s.signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a new jwt token", http.StatusInternalServerError, err)
	}
	return token, &claims, nil
}

// GenerateExt creates a new id token
func (s *service) GenerateExt(kind string, userid string, audience []string, duration int64, id string, claims interface{}) (string, error) {
	now := time.Now().Round(0)
	baseClaims := Claims{
		Claims: jwt.Claims{
			Issuer:    s.issuer,
			Subject:   userid,
			Audience:  audience,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
			ID:        id,
		},
		Kind: kind,
	}
	token, err := jwt.Signed(s.keySigner).Claims(baseClaims).Claims(claims).CompactSerialize()
	if err != nil {
		return "", governor.NewError("Failed to generate a new jwt token", http.StatusInternalServerError, err)
	}
	return token, nil
}

// HasScope returns if a token scope contains a scope
func HasScope(tokenScope string, scope string) bool {
	if scope == "" {
		return true
	}
	for _, i := range strings.Fields(tokenScope) {
		if i == ScopeAll || i == scope {
			return true
		}
	}
	return false
}

// Validate returns whether a token is valid
func (s *service) Validate(kind string, tokenString string, scope string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(s.secret, claims); err != nil {
		return false, nil
	}
	if claims.Kind != kind {
		return false, nil
	}
	if !HasScope(claims.Scope, scope) {
		return false, nil
	}
	now := time.Now().Round(0)
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: []string{s.audience},
		Time:     now,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

// GetClaims returns token claims without validating time
func (s *service) GetClaims(kind string, tokenString string, scope string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(s.secret, claims); err != nil {
		return false, nil
	}
	if claims.Kind != kind {
		return false, nil
	}
	if !HasScope(claims.Scope, scope) {
		return false, nil
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: []string{s.audience},
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

// GetClaimsExt returns external token claims without validating time
func (s *service) GetClaimsExt(kind string, tokenString string, audience []string, scope string, claims interface{}) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	if claims == nil {
		claims = &struct{}{}
	}
	baseClaims := &Claims{}
	if err := token.Claims(s.publicKey, baseClaims, claims); err != nil {
		return false, nil
	}
	if baseClaims.Kind != kind {
		return false, nil
	}
	if !HasScope(baseClaims.Scope, scope) {
		return false, nil
	}
	if err := baseClaims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: audience,
	}, 0); err != nil {
		return false, nil
	}
	return true, baseClaims
}
