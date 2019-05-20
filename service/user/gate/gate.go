package gate

import (
	"errors"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/rank"
	"github.com/labstack/echo"
	"net/http"
	"strings"
)

const (
	authenticationSubject = "authentication"
)

type (
	// Gate creates new middleware to gate routes
	Gate interface {
		Authenticate(v Validator, subject string) echo.MiddlewareFunc
	}

	gateService struct {
		tokenizer *token.Tokenizer
		baseurl   string
	}

	// Validator is a function to check the authorization of a user
	Validator func(c echo.Context, claims token.Claims) bool
)

// New returns a new Gate
func New(conf governor.Config, l governor.Logger) Gate {
	ca := conf.Conf().GetStringMapString("userauth")

	l.Info("initialize gate service", nil)

	return &gateService{
		tokenizer: token.New(ca["secret"], ca["issuer"]),
		baseurl:   conf.BaseURL,
	}
}

func getAccessCookie(c echo.Context) (string, error) {
	cookie, err := c.Cookie("access_token")
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("no cookie value")
	}
	return cookie.Value, nil
}

func rmAccessCookie(c echo.Context, baseurl string) {
	c.SetCookie(&http.Cookie{
		Name:   "access_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   baseurl,
	})
}

// Authenticate builds a middleware function to validate tokens and set claims
func (g *gateService) Authenticate(v Validator, subject string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			var accessToken string
			if t, err := getAccessCookie(c); err == nil {
				accessToken = t
			} else {
				h := strings.Split(c.Request().Header.Get("Authorization"), " ")
				if len(h) != 2 || h[0] != "Bearer" || len(h[1]) == 0 {
					return governor.NewErrorUser("User is not authorized", http.StatusUnauthorized, nil)
				}
				accessToken = h[1]
			}
			validToken, claims := g.tokenizer.Validate(accessToken, subject, "")
			if !validToken {
				rmAccessCookie(c, g.baseurl)
				return governor.NewErrorUser("User is not authorized", http.StatusUnauthorized, nil)
			}
			if !v(c, *claims) {
				return governor.NewErrorUser("User is forbidden", http.StatusForbidden, nil)
			}
			c.Set("user", claims)
			c.Set("userid", claims.Userid)
			return next(c)
		}
	}
}

// Owner is a middleware function to validate if a user owns the accessed resource
func Owner(g Gate, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagUser) && c.Param(idparam) == claims.Userid
	}, authenticationSubject)
}

// OwnerF is a middleware function to validate if a user owns the accessed resource
// idfunc should return the userid
func OwnerF(g Gate, idfunc func(echo.Context) (string, error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if !r.Has(rank.TagUser) {
			return false
		}
		s, err := idfunc(c)
		if err != nil {
			return false
		}
		return s == claims.Userid
	}, authenticationSubject)
}

// Admin is a middleware function to validate if a user is an admin
func Admin(g Gate) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// User is a middleware function to validate if the request is made by a user
func User(g Gate) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagUser)
	}, authenticationSubject)
}

// OwnerOrAdmin is a middleware function to validate if the request is made by the owner or an admin
func OwnerOrAdmin(g Gate, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagUser) && c.Param(idparam) == claims.Userid || r.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// OwnerModOrAdminF is a middleware function to validate if the request is made by the owner or a moderator
// idfunc should return the userid and the group_tag
func OwnerModOrAdminF(g Gate, idfunc func(echo.Context) (string, string, error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if r.Has(rank.TagAdmin) {
			return true
		}
		if !r.Has(rank.TagUser) {
			return false
		}
		userid, group, err := idfunc(c)
		if err != nil {
			return false
		}
		return userid == claims.Userid || r.HasMod(group)
	}, authenticationSubject)
}

// ModOrAdminF is a middleware function to validate if the request is made by the moderator of a group or an admin
// idfunc should return the group_tag
func ModOrAdminF(g Gate, idfunc func(echo.Context) (string, error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if r.Has(rank.TagAdmin) {
			return true
		}
		if !r.Has(rank.TagUser) {
			return false
		}
		s, err := idfunc(c)
		if err != nil {
			return false
		}
		return r.HasMod(s)
	}, authenticationSubject)
}

// UserOrBan is a middleware function to validate if the request is made by a user and check if the user is banned from the group
func UserOrBan(g Gate, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagUser) && !r.HasBan(c.Param(idparam))
	}, authenticationSubject)
}

// UserOrBanF is a middleware function to validate if the request is made by a user and check if the user is banned from the group
// idfunc should return the group_tag
func UserOrBanF(g Gate, idfunc func(echo.Context) (string, error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if !r.Has(rank.TagUser) {
			return false
		}
		s, err := idfunc(c)
		if err != nil {
			return false
		}
		return !r.HasBan(s)
	}, authenticationSubject)
}

// System is a middleware function to validate if the request is made by a system
func System(g Gate) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagSystem)
	}, authenticationSubject)
}
