package token

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"net/http"
	"strings"
	"time"
	"xorkevin.dev/governor"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "all"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		jwt.Claims
		Scope string `json:"scope"`
		Key   string `json:"key"`
	}

	// Tokenizer is a token generator
	Tokenizer interface {
		Generate(userid string, audience []string, duration int64, scope, id, key string) (string, *Claims, error)
		Validate(tokenString string, audience []string, scope string) (bool, *Claims)
		GetClaims(tokenString string, audience []string, scope string) (bool, *Claims)
	}

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
		logger     governor.Logger
	}
)

// New creates a new Tokenizer
func New() Service {
	return &service{
		secret:   nil,
		issuer:   "",
		audience: "",
		signer:   nil,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
	seed := sha512.Sum512(s.secret)
	if len(seed) < ed25519.SeedSize {
		return governor.NewError("ed25519 seed too small", http.StatusInternalServerError, nil)
	}
	s.privateKey = ed25519.NewKeyFromSeed(seed[:ed25519.SeedSize])
	s.publicKey = s.privateKey.Public()
	issuer := r.GetStr("issuer")
	if issuer == "" {
		return governor.NewError("Token issuer is not set", http.StatusBadRequest, nil)
	}
	s.issuer = issuer
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: s.secret}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return governor.NewError("Failed to create new jwt signer", http.StatusInternalServerError, err)
	}
	s.signer = sig
	keySig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.EdDSA, Key: s.privateKey}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return governor.NewError("Failed to create new jwt eddsa signer", http.StatusInternalServerError, err)
	}
	s.keySigner = keySig
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

// Generate returns a new jwt token from a user model
func (s *service) Generate(userid string, audience []string, duration int64, scope, id, key string) (string, *Claims, error) {
	now := time.Now().Round(0)
	if len(audience) == 0 {
		audience = []string{s.audience}
	}
	claims := Claims{
		Claims: jwt.Claims{
			Issuer:    s.issuer,
			Subject:   userid,
			Audience:  audience,
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
			ID:        id,
		},
		Scope: scope,
		Key:   key,
	}
	token, err := jwt.Signed(s.signer).Claims(claims).CompactSerialize()
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a new jwt token", http.StatusInternalServerError, err)
	}
	return token, &claims, nil
}

// Sign creates a new id token
func (s *service) Sign(userid string, audience []string, duration int64, id string, claims interface{}) (string, error) {
	now := time.Now().Round(0)
	baseClaims := jwt.Claims{
		Issuer:    s.issuer,
		Subject:   userid,
		Audience:  audience,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
		ID:        id,
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
func (s *service) Validate(tokenString string, audience []string, scope string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(s.secret, claims); err != nil {
		return false, nil
	}
	if !HasScope(claims.Scope, scope) {
		return false, nil
	}
	now := time.Now().Round(0)
	if len(audience) == 0 {
		audience = []string{s.audience}
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: audience,
		Time:     now,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

// GetClaims returns the tokens claims without validating time
func (s *service) GetClaims(tokenString string, audience []string, scope string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	claims := &Claims{}
	if err := token.Claims(s.secret, claims); err != nil {
		return false, nil
	}
	if !HasScope(claims.Scope, scope) {
		return false, nil
	}
	if len(audience) == 0 {
		audience = []string{s.audience}
	}
	if err := claims.ValidateWithLeeway(jwt.Expected{
		Issuer:   s.issuer,
		Audience: audience,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}
