package token

import (
	"context"
	"github.com/labstack/echo/v4"
	"gopkg.in/square/go-jose.v2"
	"gopkg.in/square/go-jose.v2/jwt"
	"net/http"
	"time"
	"xorkevin.dev/governor"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		Userid string `json:"userid"`
		ID     string `json:"id"`
		Key    string `json:"key"`
	}

	// Tokenizer is a token generator
	Tokenizer interface {
		Generate(userid string, duration int64, subject, id, key string) (string, *Claims, error)
		Validate(tokenString, subject string) (bool, *Claims)
		GetClaims(tokenString, subject string) (bool, *Claims)
	}

	Service interface {
		governor.Service
		Tokenizer
	}

	service struct {
		secret []byte
		issuer string
		signer jose.Signer
		logger governor.Logger
	}
)

// New creates a new Tokenizer
func New() Service {
	return &service{
		secret: nil,
		issuer: "",
		signer: nil,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	r.SetDefault("secret", "")
	r.SetDefault("issuer", "governor")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})
	secret := r.GetStr("secret")
	if secret == "" {
		return governor.NewError("Token secret is not set", http.StatusBadRequest, nil)
	}
	issuer := r.GetStr("issuer")
	if issuer == "" {
		return governor.NewError("Token issuer is not set", http.StatusBadRequest, nil)
	}
	s.secret = []byte(secret)
	s.issuer = issuer
	sig, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.HS512, Key: s.secret}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		return governor.NewError("Failed to create new jwt signer", http.StatusInternalServerError, err)
	}
	s.signer = sig
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
func (s *service) Generate(userid string, duration int64, subject, id, key string) (string, *Claims, error) {
	now := time.Now().Round(0)
	stdClaims := jwt.Claims{
		Subject:   subject,
		Issuer:    s.issuer,
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
		Expiry:    jwt.NewNumericDate(time.Unix(now.Unix()+duration, 0)),
	}
	claims := Claims{
		Userid: userid,
		ID:     id,
		Key:    key,
	}
	token, err := jwt.Signed(s.signer).Claims(stdClaims).Claims(claims).CompactSerialize()
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a new jwt token", http.StatusInternalServerError, err)
	}
	return token, &claims, nil
}

// Validate returns whether a token is valid
func (s *service) Validate(tokenString, subject string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	stdClaims := &jwt.Claims{}
	claims := &Claims{}
	if err := token.Claims(s.secret, stdClaims, claims); err != nil {
		return false, nil
	}
	if err := stdClaims.ValidateWithLeeway(jwt.Expected{
		Subject: subject,
		Issuer:  s.issuer,
	}, 0); err != nil {
		return false, nil
	}
	return true, claims
}

// GetClaims returns the tokens claims without validating time
func (s *service) GetClaims(tokenString, subject string) (bool, *Claims) {
	token, err := jwt.ParseSigned(tokenString)
	if err != nil {
		return false, nil
	}
	stdClaims := &jwt.Claims{}
	claims := &Claims{}
	if err := token.Claims(s.secret, stdClaims, claims); err != nil {
		return false, nil
	}
	if stdClaims.Subject != subject || stdClaims.Issuer != s.issuer {
		return false, nil
	}
	return true, claims
}
