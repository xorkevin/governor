package token

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/model"
	"net/http"
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
)

const (
	moduleToken = "token"
)

// New returns a new jwt token from a user model
func New(user *usermodel.Model) (string, *governor.Error) {
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS512, Claims{}).SignedString("")
	if err != nil {
		return "", governor.NewError(moduleToken, err.Error(), 0, http.StatusInternalServerError)
	}
	return token, nil
}
