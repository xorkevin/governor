package gate

import (
	"context"
	"errors"
	"github.com/go-chi/chi"
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
		Authenticate(v Validator, subject string) governor.Middleware
	}

	// Gate creates new middleware to gate routes
	Gate interface {
		Authenticator
		WithApikey() Authenticator
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
		Request() *http.Request
	}

	intersector struct {
		s      *service
		userid string
		req    *http.Request
	}

	apikeyIntersector struct {
		s        *apikeyAuth
		apikeyid string
		userid   string
		req      *http.Request
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

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
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

func getAccessCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("access_token")
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errors.New("no cookie value")
	}
	return cookie.Value, nil
}

func rmAccessCookie(w http.ResponseWriter, baseurl string) {
	http.SetCookie(w, &http.Cookie{
		Name:   "access_token",
		Value:  "invalid",
		MaxAge: -1,
		Path:   baseurl,
	})
}

func (r *intersector) Userid() string {
	return r.userid
}

func (r *intersector) Request() *http.Request {
	return r.req
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

func (s *service) intersector(userid string, req *http.Request) Intersector {
	return &intersector{
		s:      s,
		userid: userid,
		req:    req,
	}
}

// Authenticate builds a middleware function to validate tokens and set claims
func (s *service) Authenticate(v Validator, subject string) governor.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, s.logger)
			accessToken, err := getAccessCookie(r)
			if err != nil {
				h := strings.Split(r.Header.Get("Authorization"), " ")
				if len(h) != 2 || h[0] != "Bearer" || len(h[1]) == 0 {
					c.WriteError(governor.NewErrorUser("User is not authorized", http.StatusUnauthorized, nil))
					return
				}
				accessToken = h[1]
			}
			validToken, claims := s.tokenizer.Validate(accessToken, subject)
			if !validToken {
				rmAccessCookie(w, s.baseurl)
				c.WriteError(governor.NewErrorUser("User is not authorized", http.StatusUnauthorized, nil))
				return
			}
			if !v(s.intersector(claims.Userid, r)) {
				c.WriteError(governor.NewErrorUser("User is forbidden", http.StatusForbidden, nil))
				return
			}
			ctx := context.WithValue(r.Context(), "userid", claims.Userid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (s *service) WithApikey() Authenticator {
	return &apikeyAuth{
		base:    s,
		roles:   s.roles,
		apikeys: s.apikeys,
		logger:  s.logger,
	}
}

func (r *apikeyIntersector) Userid() string {
	return r.userid
}

func (r *apikeyIntersector) Request() *http.Request {
	return r.req
}

func (r *apikeyIntersector) Intersect(roles rank.Rank) (rank.Rank, bool) {
	k, err := r.s.apikeys.IntersectRoles(r.apikeyid, roles)
	if err != nil {
		r.s.logger.Error("Failed to get apikey roles", map[string]string{
			"error":      err.Error(),
			"actiontype": "authgetapikeyroles",
		})
		return nil, false
	}
	return k, true
}

func (s *apikeyAuth) intersector(apikeyid, userid string, req *http.Request) Intersector {
	return &apikeyIntersector{
		s:        s,
		apikeyid: apikeyid,
		userid:   userid,
		req:      req,
	}
}

func (s *apikeyAuth) Authenticate(v Validator, subject string) governor.Middleware {
	middle := s.base.Authenticate(v, subject)
	return func(next http.Handler) http.Handler {
		base := middle(next)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			keyid, password, ok := r.BasicAuth()
			if !ok {
				base.ServeHTTP(w, r)
				return
			}
			c := governor.NewContext(w, r, s.logger)
			userid, err := s.apikeys.CheckKey(keyid, password)
			if err != nil {
				w.Header().Add("WWW-Authenticate", "Basic realm=\"governor\"")
				c.WriteError(governor.NewErrorUser("User is not authorized", http.StatusUnauthorized, nil))
				return
			}
			if !v(s.intersector(keyid, userid, r)) {
				c.WriteError(governor.NewErrorUser("User is forbidden", http.StatusForbidden, nil))
				return
			}
			ctx := context.WithValue(r.Context(), "userid", userid)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Owner is a middleware function to validate if a user owns the resource
//
// idfunc should return true if the resource is owned by the given user
func Owner(g Authenticator, idfunc func(*http.Request, string) bool) governor.Middleware {
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
		return idfunc(r.Request(), r.Userid())
	}, authenticationSubject)
}

// OwnerParam is a middleware function to validate if a url param is the given
// userid
func OwnerParam(g Authenticator, idparam string) governor.Middleware {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return Owner(g, func(r *http.Request, userid string) bool {
		return chi.URLParam(r, idparam) == userid
	})
}

// Admin is a middleware function to validate if a user is an admin
func Admin(g Authenticator) governor.Middleware {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin}))
		if !ok {
			return false
		}
		return roles.Has(rank.TagAdmin)
	}, authenticationSubject)
}

// User is a middleware function to validate if a user is authenticated and not
// banned
func User(g Authenticator) governor.Middleware {
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
// the resource owner or an admin
//
// idfunc should return true if the resource is owned by the given user
func OwnerOrAdmin(g Authenticator, idfunc func(*http.Request, string) bool) governor.Middleware {
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
		return idfunc(r.Request(), r.Userid())
	}, authenticationSubject)
}

// OwnerOrAdminParam is a middleware function to validate if a url param is the
// given userid or if the user is an admin
func OwnerOrAdminParam(g Authenticator, idparam string) governor.Middleware {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return OwnerOrAdmin(g, func(r *http.Request, userid string) bool {
		return chi.URLParam(r, idparam) == userid
	})
}

// ModF is a middleware function to validate if the request is made by the
// moderator of a group or an admin
//
// idfunc should return the group of the resource
func ModF(g Authenticator, idfunc func(*http.Request, string) (string, error)) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		modtag, err := idfunc(r.Request(), r.Userid())
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

// Mod is a middleware function to validate if the request is made by a
// moderator of the group or an admin
func Mod(g Authenticator, group string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return ModF(g, func(_ *http.Request, _ string) (string, error) {
		return group, nil
	})
}

// NoBanF is a middleware function to validate if the request is made by a user
// not banned from the group
//
// idfunc should return the group of the resource
func NoBanF(g Authenticator, idfunc func(*http.Request, string) (string, error)) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		bantag, err := idfunc(r.Request(), r.Userid())
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

// NoBan is a middleware function to validate if the request is made by a
// user not banned from the group
func NoBan(g Authenticator, group string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return NoBanF(g, func(_ *http.Request, _ string) (string, error) {
		return group, nil
	})
}

// MemberF is a middleware function to validate if the request is made by a
// member of a group
//
// idfunc should return the group of the resource
func MemberF(g Authenticator, idfunc func(*http.Request, string) (string, error)) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		tag, err := idfunc(r.Request(), r.Userid())
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

// Member is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
func Member(g Authenticator, group string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return MemberF(g, func(_ *http.Request, _ string) (string, error) {
		return group, nil
	})
}

// System is a middleware function to validate if the request is made by a system
func System(g Authenticator) governor.Middleware {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.System())
		if !ok {
			return false
		}
		return roles.Has(rank.TagSystem)
	}, authenticationSubject)
}
