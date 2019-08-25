package token

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"net/http"
	"time"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/model"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		jwt.StandardClaims
		Userid   string `json:"userid"`
		AuthTags string `json:"auth_tags"`
		ID       string `json:"id"`
		Key      string `json:"key"`
	}

	// Tokenizer is a token generator
	Tokenizer struct {
		secret []byte
		issuer string
		parser *jwt.Parser
	}
)

// New creates a new Tokenizer
func New(secret, issuer string) *Tokenizer {
	return &Tokenizer{
		secret: []byte(secret),
		issuer: issuer,
		parser: &jwt.Parser{},
	}
}

// Generate returns a new jwt token from a user model
func (t *Tokenizer) Generate(u *usermodel.Model, duration int64, subject, id, key string) (string, *Claims, error) {
	now := time.Now().Unix()
	claims := &Claims{
		StandardClaims: jwt.StandardClaims{
			Subject:   subject,
			Issuer:    t.issuer,
			IssuedAt:  now,
			ExpiresAt: now + duration,
		},
		Userid:   u.Userid,
		AuthTags: u.AuthTags.Stringify(),
		ID:       id,
		Key:      key,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(t.secret)
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a new jwt token", http.StatusInternalServerError, err)
	}
	return token, claims, nil
}

// GenerateFromClaims creates a new jwt from a set of claims
func (t *Tokenizer) GenerateFromClaims(claims *Claims, duration int64, subject, key string) (string, *Claims, error) {
	now := time.Now().Unix()
	claims.Subject = subject
	claims.IssuedAt = now
	claims.ExpiresAt = now + duration
	claims.Key = key
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(t.secret)
	if err != nil {
		return "", nil, governor.NewError("Failed to generate a jwt from claims", http.StatusInternalServerError, err)
	}
	return token, claims, nil
}

// Validate returns whether a token is valid
func (t *Tokenizer) Validate(tokenString, subject string) (bool, *Claims) {
	if token, err := t.parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return t.secret, nil
	}); err == nil {
		if claims, ok := token.Claims.(*Claims); ok {
			if claims.Valid() == nil && claims.VerifyIssuer(t.issuer, true) && claims.Subject == subject {
				return true, claims
			}
		}
	}
	return false, nil
}

// GetClaims returns the tokens claims without validation
func (t *Tokenizer) GetClaims(tokenString, subject string) (bool, *Claims) {
	if token, err := t.parser.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return t.secret, nil
	}); err == nil {
		if claims, ok := token.Claims.(*Claims); ok {
			if claims.VerifyIssuer(t.issuer, true) && claims.Subject == subject {
				return true, claims
			}
		}
	}
	return false, nil
}
