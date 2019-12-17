package gate

import (
	"context"
	"errors"
	"github.com/labstack/echo/v4"
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

const (
	authenticationSubject = "authentication"
)

type (
	// Gate creates new middleware to gate routes
	Gate interface {
		Authenticate(v Validator, subject string) echo.MiddlewareFunc
	}

	Service interface {
		governor.Service
		Gate
	}

	service struct {
		tokenizer *token.Tokenizer
		baseurl   string
		logger    governor.Logger
	}

	// Validator is a function to check the authorization of a user
	Validator func(c echo.Context, claims token.Claims) bool

	Claims struct {
		token.Claims
	}
)

// New returns a new Gate
func New() Service {
	return &service{}
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

	if r.GetStr("secret") == "" {
		l.Warn("token secret is not set", nil)
	}
	if r.GetStr("issuer") == "" {
		l.Warn("token issuer is not set", nil)
	}
	s.tokenizer = token.New(r.GetStr("secret"), r.GetStr("issuer"))
	s.baseurl = c.BaseURL
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
func (s *service) Authenticate(v Validator, subject string) echo.MiddlewareFunc {
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
			validToken, claims := s.tokenizer.Validate(accessToken, subject)
			if !validToken {
				rmAccessCookie(c, s.baseurl)
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

// Owner is a middleware function to validate if a user owns the accessed
// resource
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

// OwnerF is a middleware function to validate if a user owns the accessed
// resource
//
// idfunc should return the userid
func OwnerF(g Gate, idfunc func(echo.Context, Claims) (string, error)) echo.MiddlewareFunc {
	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if !r.Has(rank.TagUser) {
			return false
		}
		s, err := idfunc(c, Claims{claims})
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
		if r.Has(rank.TagAdmin) {
			return true
		}
		return r.Has(rank.TagUser)
	}, authenticationSubject)
}

// OwnerOrAdmin is a middleware function to validate if the request is made by
// the owner or an admin
func OwnerOrAdmin(g Gate, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if r.Has(rank.TagAdmin) {
			return true
		}
		return r.Has(rank.TagUser) && c.Param(idparam) == claims.Userid
	}, authenticationSubject)
}

// ModOrAdminF is a middleware function to validate if the request is made by
// the moderator of a group or an admin
//
// idfunc should return the group_tag
func ModOrAdminF(g Gate, idfunc func(echo.Context, Claims) (string, error)) echo.MiddlewareFunc {
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
		s, err := idfunc(c, Claims{claims})
		if err != nil {
			return false
		}
		return r.HasMod(s)
	}, authenticationSubject)
}

// UserOrBan is a middleware function to validate if the request is made by a
// user and check if the user is banned from the group
func UserOrBan(g Gate, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(c echo.Context, claims token.Claims) bool {
		r, err := rank.FromStringUser(claims.AuthTags)
		if err != nil {
			return false
		}
		if r.Has(rank.TagAdmin) {
			return true
		}
		return r.Has(rank.TagUser) && !r.HasBan(c.Param(idparam))
	}, authenticationSubject)
}

// UserOrBanF is a middleware function to validate if the request is made by a
// user and check if the user is banned from the group
//
// idfunc should return the group_tag
func UserOrBanF(g Gate, idfunc func(echo.Context, Claims) (string, error)) echo.MiddlewareFunc {
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
		s, err := idfunc(c, Claims{claims})
		if err != nil {
			return false
		}
		return !r.HasBan(s)
	}, authenticationSubject)
}

// Member is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
func Member(g Gate, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}
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
		return r.HasUser(c.Param(idparam)) && !r.HasBan(c.Param(idparam))
	}, authenticationSubject)
}

// MemberF is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
//
// idfunc should return the group_tag
func MemberF(g Gate, idfunc func(echo.Context, Claims) (string, error)) echo.MiddlewareFunc {
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
		s, err := idfunc(c, Claims{claims})
		if err != nil {
			return false
		}
		return r.HasUser(s) && !r.HasBan(s)
	}, authenticationSubject)
}

// OwnerOrMemberF is a middleware function to validate if the request is made
// by the owner or a moderator
//
// idfunc should return the userid and the group_tag
func OwnerOrMemberF(g Gate, idfunc func(echo.Context, Claims) (string, string, error)) echo.MiddlewareFunc {
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
		userid, group, err := idfunc(c, Claims{claims})
		if err != nil {
			return false
		}
		return userid == claims.Userid || r.HasUser(group)
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
