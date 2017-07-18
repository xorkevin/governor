package gate

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/user/token"
	"github.com/hackform/governor/util/rank"
	"github.com/labstack/echo"
	"net/http"
	"strings"
)

const (
	moduleID              = "user.middleware"
	moduleIDAuth          = moduleID + ".gate"
	authenticationSubject = "authentication"
)

type (
	// Gate creates new middleware to gate routes
	Gate struct {
		tokenizer *token.Tokenizer
	}
	// Validator is a function to check the authorization of a user
	Validator func(c echo.Context, claims token.Claims) bool
)

// New returns a new Gate
func New(secret, issuer string) *Gate {
	return &Gate{
		tokenizer: token.New(secret, issuer),
	}
}

// Authenticate builds a middleware function to validate tokens and set claims
func (g *Gate) Authenticate(v Validator, subject string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := strings.Split(c.Request().Header.Get("Authorization"), " ")
			if len(h) != 2 || h[0] != "Bearer" || len(h[1]) == 0 {
				return governor.NewErrorUser(moduleIDAuth, "user is not authorized", 0, http.StatusUnauthorized)
			}
			validToken, claims := g.tokenizer.Validate(h[1], subject, "")
			if !validToken {
				return governor.NewErrorUser(moduleIDAuth, "user is not authorized", 0, http.StatusUnauthorized)
			}
			if !v(c, *claims) {
				return governor.NewErrorUser(moduleIDAuth, "user is forbidden", 0, http.StatusForbidden)
			}
			c.Set("user", claims)
			return next(c)
		}
	}
}

// Owner is a middleware function to validate if a user owns the accessed resource
func (g *Gate) Owner(idparam string) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		return c.Param(idparam) == claims.Userid
	}, authenticationSubject)
}

// OwnerF is a middleware function to validate if a user owns the accessed resource
func (g *Gate) OwnerF(idparam string, idfunc func(string) (string, *governor.Error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		s, err := idfunc(c.Param(idparam))
		if err != nil {
			return false
		}
		return s == claims.Userid
	}, authenticationSubject)
}

// Admin is a middleware function to validate if a user is an admin
func (g *Gate) Admin() echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// User is a middleware function to validate if the request is made by a user
func (g *Gate) User() echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagUser)
	}, authenticationSubject)
}

// OwnerOrAdmin is a middleware function to validate if the request is made by the owner or an admin
func (g *Gate) OwnerOrAdmin(idparam string) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return c.Param(idparam) == claims.Userid || r.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// OwnerOrAdminF is a middleware function to validate if the request is made by the owner or an admin
func (g *Gate) OwnerOrAdminF(idparam string, idfunc func(string) (string, *governor.Error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		s, err := idfunc(c.Param(idparam))
		if err != nil {
			return false
		}
		return s == claims.Userid || r.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// UserOrBan is a middleware function to validate if the request is made a user and check if the user is banned from the group
func (g *Gate) UserOrBan(idparam string) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagUser) && !r.HasBan(c.Param(idparam))
	}, authenticationSubject)
}

// System is a middleware function to validate if the request is made by a system
func (g *Gate) System() echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		return r.Has(rank.TagSystem)
	}, authenticationSubject)
}
