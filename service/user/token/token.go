package token

import (
	"context"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo/v4"
	"net/http"
	"time"
	"xorkevin.dev/governor"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		jwt.StandardClaims
		Userid string `json:"userid"`
		ID     string `json:"id"`
		Key    string `json:"key"`
	}

	// Tokenizer is a token generator
	Tokenizer interface {
		Generate(userid string, duration int64, subject, id, key string) (string, *Claims, error)
		GenerateFromClaims(claims *Claims, duration int64, subject, key string) (string, *Claims, error)
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
		parser *jwt.Parser
		logger governor.Logger
	}
)

// New creates a new Tokenizer
func New() Service {
	return &service{
		secret: nil,
		issuer: "",
		parser: &jwt.Parser{},
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
		l.Warn("token secret is not set", nil)
	}
	issuer := r.GetStr("issuer")
	if issuer == "" {
		l.Warn("token issuer is not set", nil)
	}
	s.secret = []byte(secret)
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

// Generate returns a new jwt token from a user model
func (s *service) Generate(userid string, duration int64, subject, id, key string) (string, *Claims, error) {
	now := time.Now().Round(0).Unix()
	claims := &Claims{
		StandardClaims: jwt.StandardClaims{
			Subject:   subject,
			Issuer:    s.issuer,
			IssuedAt:  now,
			ExpiresAt: now + duration,
		},
		Userid: userid,
		ID:     id,
		Key:    key,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(s.secret)
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a new jwt token", http.StatusInternalServerError, err)
	}
	return token, claims, nil
}

// GenerateFromClaims creates a new jwt from a set of claims
func (s *service) GenerateFromClaims(claims *Claims, duration int64, subject, key string) (string, *Claims, error) {
	now := time.Now().Round(0).Unix()
	claims.Subject = subject
	claims.IssuedAt = now
	claims.ExpiresAt = now + duration
	claims.Key = key
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(s.secret)
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a jwt from claims", http.StatusInternalServerError, err)
	}
	return token, claims, nil
}

// Validate returns whether a token is valid
func (s *service) Validate(tokenString, subject string) (bool, *Claims) {
	if token, err := s.parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	}); err == nil {
		if claims, ok := token.Claims.(*Claims); ok {
			if claims.Valid() == nil && claims.VerifyIssuer(s.issuer, true) && claims.Subject == subject {
				return true, claims
			}
		}
	}
	return false, nil
}

// GetClaims returns the tokens claims without validating time
func (s *service) GetClaims(tokenString, subject string) (bool, *Claims) {
	if token, err := s.parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return s.secret, nil
	}); err == nil {
		if claims, ok := token.Claims.(*Claims); ok {
			if claims.VerifyIssuer(s.issuer, true) && claims.Subject == subject {
				return true, claims
			}
		}
	}
	return false, nil
}
