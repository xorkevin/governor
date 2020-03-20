package gate

import (
	"context"
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"net/http"
	"strings"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

const (
	authenticationSubject = "authentication"
)

type (
	// Authenticator creates new authenticating middleware
	Authenticator interface {
		Authenticate(v Validator, subject string) echo.MiddlewareFunc
	}

	// Gate creates new middleware to gate routes
	Gate interface {
		Authenticator
	}

	Service interface {
		governor.Service
		Gate
	}

	service struct {
		roles     role.Role
		apikeys   apikey.Apikey
		tokenizer token.Tokenizer
		baseurl   string
		logger    governor.Logger
	}

	apikeyAuth struct {
		base    Authenticator
		roles   role.Role
		apikeys apikey.Apikey
		logger  governor.Logger
	}

	// Intersector is a function that returns roles needed to validate a user
	Intersector interface {
		Userid() string
		Intersect(roles rank.Rank) (rank.Rank, bool)
		Context() echo.Context
	}

	intersector struct {
		s      *service
		userid string
		ctx    echo.Context
	}

	apikeyIntersector struct {
		s        *apikeyAuth
		apikeyid string
		userid   string
		ctx      echo.Context
	}

	// Validator is a function to check the authorization of a user
	Validator func(r Intersector) bool
)

// New returns a new Gate
func New(roles role.Role, apikeys apikey.Apikey, tokenizer token.Tokenizer) Service {
	return &service{
		roles:     roles,
		apikeys:   apikeys,
		tokenizer: tokenizer,
	}
}

func (s *service) Register(r governor.ConfigRegistrar, jr governor.JobRegistrar) {
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, g *echo.Group) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})
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

func (r *intersector) Userid() string {
	return r.userid
}

func (r *intersector) Context() echo.Context {
	return r.ctx
}

func (r *intersector) Intersect(roles rank.Rank) (rank.Rank, bool) {
	k, err := r.s.roles.IntersectRoles(r.userid, roles)
	if err != nil {
		r.s.logger.Error("Failed to get user roles", map[string]string{
			"error":      err.Error(),
			"actiontype": "authgetroles",
		})
		return nil, false
	}
	return k, true
}

func (s *service) intersector(userid string, c echo.Context) Intersector {
	return &intersector{
		s:      s,
		userid: userid,
		ctx:    c,
	}
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
			if !v(s.intersector(claims.Userid, c)) {
				return governor.NewErrorUser("User is forbidden", http.StatusForbidden, nil)
			}
			c.Set("userid", claims.Userid)
			return next(c)
		}
	}
}

func (r *apikeyIntersector) Userid() string {
	return r.userid
}

func (r *apikeyIntersector) Context() echo.Context {
	return r.ctx
}

func (r *apikeyIntersector) Intersect(roles rank.Rank) (rank.Rank, bool) {
	// TODO: intersect apikey roles as well
	k, err := r.s.roles.IntersectRoles(r.userid, roles)
	if err != nil {
		r.s.logger.Error("Failed to get user roles", map[string]string{
			"error":      err.Error(),
			"actiontype": "authgetroles",
		})
		return nil, false
	}
	return k, true
}

func (s *apikeyAuth) intersector(apikeyid, userid string, c echo.Context) Intersector {
	return &apikeyIntersector{
		s:        s,
		apikeyid: apikeyid,
		userid:   userid,
		ctx:      c,
	}
}

const (
	basicAuthRealm  = "governor"
	basicAuthHeader = "Authorization"
	basicAuthType   = "basic"
)

func (s *apikeyAuth) Authenticate(v Validator, subject string) echo.MiddlewareFunc {
	basicAuth := middleware.BasicAuthWithConfig(middleware.BasicAuthConfig{
		Skipper: middleware.DefaultSkipper,
		Validator: func(keyid, password string, c echo.Context) (bool, error) {
			k := strings.SplitN(keyid, "|", 2)
			if len(k) != 2 {
				return false, governor.NewErrorUser("Invalid apikey id", http.StatusUnauthorized, nil)
			}
			userid := k[0]
			// TODO: validate apikey password
			if !v(s.intersector(keyid, userid, c)) {
				return false, governor.NewErrorUser("User is forbidden", http.StatusForbidden, nil)
			}
			c.Set("userid", userid)
			return true, nil
		},
		Realm: basicAuthRealm,
	})
	middle := s.base.Authenticate(v, subject)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		basicAuthHandle := basicAuth(next)
		handleBase := middle(next)
		return func(c echo.Context) error {
			if l, header := len(basicAuthType), c.Request().Header.Get(basicAuthHeader); len(header) > l+1 && strings.ToLower(header[:l]) == basicAuthType {
				return basicAuthHandle(c)
			}
			return handleBase(c)
		}
	}
}

// Owner is a middleware function to validate if a user owns the accessed
// resource
func Owner(g Authenticator, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagUser}))
		if !ok {
			return false
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return r.Context().Param(idparam) == r.Userid()
	}, authenticationSubject)
}

// OwnerF is a middleware function to validate if a user owns the accessed
// resource
//
// idfunc should return the userid
func OwnerF(g Authenticator, idfunc func(echo.Context, string) (string, error)) echo.MiddlewareFunc {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagUser}))
		if !ok {
			return false
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		userid := r.Userid()
		s, err := idfunc(r.Context(), userid)
		if err != nil {
			return false
		}
		return s == userid
	}, authenticationSubject)
}

// Admin is a middleware function to validate if a user is an admin
func Admin(g Authenticator) echo.MiddlewareFunc {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin}))
		if !ok {
			return false
		}
		return roles.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// User is a middleware function to validate if the request is made by a user
func User(g Authenticator) echo.MiddlewareFunc {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		return roles.Has(rank.TagUser)
	}, authenticationSubject)
}

// OwnerOrAdmin is a middleware function to validate if the request is made by
// the owner or an admin
func OwnerOrAdmin(g Authenticator, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return r.Context().Param(idparam) == r.Userid()
	}, authenticationSubject)
}

// OwnerOrAdminF is a middleware function to validate if the request is made by
// the owner or an admin
//
// idfunc should return the userid
func OwnerOrAdminF(g Authenticator, idfunc func(echo.Context, string) (string, error)) echo.MiddlewareFunc {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		userid := r.Userid()
		s, err := idfunc(r.Context(), userid)
		if err != nil {
			return false
		}
		return s == userid
	}, authenticationSubject)
}

// Mod is a middleware function to validate if the request is made by the
// moderator of a group or an admin
func Mod(g Authenticator, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(r Intersector) bool {
		modtag := r.Context().Param(idparam)
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddMod(modtag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return roles.HasMod(modtag)
	}, authenticationSubject)
}

// ModF is a middleware function to validate if the request is made by the
// moderator of a group or an admin
//
// idfunc should return the group_tag
func ModF(g Authenticator, idfunc func(echo.Context, string) (string, error)) echo.MiddlewareFunc {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		modtag, err := idfunc(r.Context(), r.Userid())
		if err != nil {
			return false
		}
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddMod(modtag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return roles.HasMod(modtag)
	}, authenticationSubject)
}

// UserOrBan is a middleware function to validate if the request is made by a
// user and check if the user is banned from the group
func UserOrBan(g Authenticator, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(r Intersector) bool {
		bantag := r.Context().Param(idparam)
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddBan(bantag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return !roles.HasBan(bantag)
	}, authenticationSubject)
}

// UserOrBanF is a middleware function to validate if the request is made by a
// user and check if the user is banned from the group
//
// idfunc should return the group_tag
func UserOrBanF(g Authenticator, idfunc func(echo.Context, string) (string, error)) echo.MiddlewareFunc {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		bantag, err := idfunc(r.Context(), r.Userid())
		if err != nil {
			return false
		}
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddBan(bantag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return !roles.HasBan(bantag)
	}, authenticationSubject)
}

// Member is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
func Member(g Authenticator, idparam string) echo.MiddlewareFunc {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return g.Authenticate(func(r Intersector) bool {
		tag := r.Context().Param(idparam)
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddUser(tag).AddBan(tag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return roles.HasUser(tag) && !roles.HasBan(tag)
	}, authenticationSubject)
}

// MemberF is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
//
// idfunc should return the group_tag
func MemberF(g Authenticator, idfunc func(echo.Context, string) (string, error)) echo.MiddlewareFunc {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		tag, err := idfunc(r.Context(), r.Userid())
		if err != nil {
			return false
		}
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddUser(tag).AddBan(tag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return roles.HasUser(tag) && !roles.HasBan(tag)
	}, authenticationSubject)
}

// OwnerOrMemberF is a middleware function to validate if the request is made
// by the owner or a group member
//
// idfunc should return the userid and the group_tag
func OwnerOrMemberF(g Authenticator, idfunc func(echo.Context, string) (string, string, error)) echo.MiddlewareFunc {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		userid := r.Userid()
		s, tag, err := idfunc(r.Context(), userid)
		if err != nil {
			return false
		}
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}).AddUser(tag).AddBan(tag))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return s == userid || (roles.HasUser(tag) && !roles.HasBan(tag))
	}, authenticationSubject)
}

// System is a middleware function to validate if the request is made by a system
func System(g Authenticator) echo.MiddlewareFunc {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.System())
		if !ok {
			return false
		}
		return roles.Has(rank.TagSystem)
	}, authenticationSubject)
}
