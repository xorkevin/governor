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
		secret []byte
		issuer string
	}
)

const (
	moduleToken = "token"
)

// New creates a new Tokenizer
func New(secret, issuer string) *Tokenizer {
	return &Tokenizer{
		secret: []byte(secret),
		issuer: issuer,
	}
}

const (
	moduleTokenGenerate = moduleToken + ".generate"
)

// Generate returns a new jwt token from a user model
func (t *Tokenizer) Generate(u *usermodel.Model, duration int64, subject, id string) (string, *Claims, *governor.Error) {
	userid, err := u.IDBase64()
	if err != nil {
		err.AddTrace(moduleTokenGenerate)
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
		Userid:    userid,
		Username:  u.Username,
		AuthTags:  u.Auth.Tags,
		Email:     u.Email,
		FirstName: u.FirstName,
		LastName:  u.LastName,
	}
	token, errjwt := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString(t.secret)
	if errjwt != nil {
		return "", nil, governor.NewError(moduleTokenGenerate, errjwt.Error(), 0, http.StatusInternalServerError)
	}
	return token, claims, nil
}

const (
	moduleTokenValidate = moduleToken + ".validate"
)

// Validate returns whether a token is valid or not
func (t *Tokenizer) Validate(tokenString, subject, id string) (bool, *Claims) {
	if token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) { return t.secret, nil }); err == nil {
		if claims, ok := token.Claims.(*Claims); ok {
			if claims.Valid() == nil && claims.VerifyIssuer(t.issuer, true) && claims.Subject == subject && claims.Id == id {
				return true, claims
			}
		}
	}
	return false, nil
}
