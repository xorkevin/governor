package token

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"net/http"
	"time"
)

type (
	// Claims is a set of fields to describe a user
	Claims struct {
		jwt.StandardClaims
		Userid    string `json:"userid"`
		Username  string `json:"username"`
		AuthTags  string `json:"auth_tags"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}

	// Tokenizer is a token generator
	Tokenizer struct {
		secret string
		issuer string
	}
)

const (
	moduleToken = "token"
)

// New creates a new Tokenizer
func New(secret, issuer string) *Tokenizer {
	return &Tokenizer{
		secret: secret,
		issuer: issuer,
	}
}

// Generate returns a new jwt token from a user model
func (t *Tokenizer) Generate(u *usermodel.Model, duration int64, subject, id string) (string, *Claims, *governor.Error) {
	id, err := u.IDBase64()
	if err != nil {
		return "", nil, err
	}
	now := time.Now().Unix()
	claims := &Claims{
		StandardClaims: jwt.StandardClaims{
			Subject:   subject,
			Id:        id,
			Issuer:    t.issuer,
			IssuedAt:  now,
			ExpiresAt: now + duration,
		},
		Userid:    id,
		Username:  u.Username,
		AuthTags:  u.Auth.Tags,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
	}
	token, errjwt := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(t.secret)
	if errjwt != nil {
		return "", nil, governor.NewError(moduleToken, errjwt.Error(), 0, http.StatusInternalServerError)
	}
	return token, claims, nil
}
