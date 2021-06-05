package gate

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/user/apikey"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/util/rank"
)

type (
	ctxKeyUserid struct{}
	ctxKeyClaims struct{}
)

// GetCtxUserid returns a userid from the context
func GetCtxUserid(c governor.Context) string {
	v := c.Get(ctxKeyUserid{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxUserid(c governor.Context, userid string) {
	c.Set(ctxKeyUserid{}, userid)
}

// GetCtxClaims returns token claims from the context
func GetCtxClaims(c governor.Context) *token.Claims {
	v := c.Get(ctxKeyClaims{})
	if v == nil {
		return nil
	}
	return v.(*token.Claims)
}

func setCtxClaims(c governor.Context, claims *token.Claims) {
	c.Set(ctxKeyClaims{}, claims)
}

type (
	// Gate creates new authenticating middleware
	Gate interface {
		Authenticate(v Validator, scope string) governor.Middleware
	}

	// Service is a Gate and governor.Service
	Service interface {
		governor.Service
		Gate
	}

	service struct {
		roles     role.Roles
		apikeys   apikey.Apikeys
		tokenizer token.Tokenizer
		baseurl   string
		realm     string
		logger    governor.Logger
	}

	// Intersector is a function that returns roles needed to validate a user
	Intersector interface {
		Userid() string
		Intersect(roles rank.Rank) (rank.Rank, bool)
		HasScope(scope string) bool
		Ctx() governor.Context
	}

	intersector struct {
		s      *service
		userid string
		scope  string
		ctx    governor.Context
	}

	// Validator is a function to check the authorization of a user
	Validator func(r Intersector) bool

	ctxKeyGate struct{}
)

// GetCtxGate returns a Gate from the context
func GetCtxGate(inj governor.Injector) Gate {
	v := inj.Get(ctxKeyGate{})
	if v == nil {
		return nil
	}
	return v.(Gate)
}

// setCtxGate sets a Gate in the context
func setCtxGate(inj governor.Injector, g Gate) {
	inj.Set(ctxKeyGate{}, g)
}

// NewCtx creates a new Gate from a context
func NewCtx(inj governor.Injector) Service {
	roles := role.GetCtxRoles(inj)
	apikeys := apikey.GetCtxApikeys(inj)
	tokenizer := token.GetCtxTokenizer(inj)
	return New(roles, apikeys, tokenizer)
}

// New returns a new Gate
func New(roles role.Roles, apikeys apikey.Apikeys, tokenizer token.Tokenizer) Service {
	return &service{
		roles:     roles,
		apikeys:   apikeys,
		tokenizer: tokenizer,
	}
}

func (s *service) Register(inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
	setCtxGate(inj, s)

	r.SetDefault("realm", "governor")
}

func (s *service) Init(ctx context.Context, c governor.Config, r governor.ConfigReader, l governor.Logger, m governor.Router) error {
	s.logger = l
	l = s.logger.WithData(map[string]string{
		"phase": "init",
	})
	s.baseurl = c.BaseURL
	s.realm = r.GetStr("realm")
	l.Info("loaded config", map[string]string{
		"realm": s.realm,
	})
	return nil
}

func (s *service) Setup(req governor.ReqSetup) error {
	return nil
}

func (s *service) PostSetup(req governor.ReqSetup) error {
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

type (
	errInvalidHeader struct{}
	errAuthNotFound  struct{}
)

func (e errInvalidHeader) Error() string {
	return "Invalid auth header"
}

func (e errAuthNotFound) Error() string {
	return "Auth not found"
}

func getAccessCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("access_token")
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", errAuthNotFound{}
	}
	return cookie.Value, nil
}

func getAuthHeader(c governor.Context) (string, error) {
	authHeader := c.Header("Authorization")
	if authHeader == "" {
		return "", errAuthNotFound{}
	}
	h := strings.SplitN(authHeader, " ", 2)
	if len(h) != 2 || h[0] != "Bearer" || len(h[1]) == 0 {
		return "", errInvalidHeader{}
	}
	token := h[1]
	if token == "" {
		return "", errInvalidHeader{}
	}
	return token, nil
}

func (r *intersector) Userid() string {
	return r.userid
}

func (r *intersector) Ctx() governor.Context {
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

func (r *intersector) HasScope(scope string) bool {
	return token.HasScope(r.scope, scope)
}

func (s *service) intersector(userid string, scope string, ctx governor.Context) Intersector {
	return &intersector{
		s:      s,
		userid: userid,
		scope:  scope,
		ctx:    ctx,
	}
}

const (
	oauthErrorInvalidToken      = "invalid_token"
	oauthErrorInsufficientScope = "insufficient_scope"
)

// Authenticate builds a middleware function to validate tokens and set claims
func (s *service) Authenticate(v Validator, scope string) governor.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, s.logger)
			keyid, password, ok := r.BasicAuth()
			if ok {
				userid, keyscope, err := s.apikeys.CheckKey(keyid, password)
				if err != nil {
					if !errors.Is(err, apikey.ErrInvalidKey{}) && !errors.Is(err, apikey.ErrNotFound{}) {
						c.WriteError(governor.ErrWithMsg(err, "Failed to get apikey"))
						return
					}
					c.SetHeader(
						"WWW-Authenticate",
						fmt.Sprintf(
							`Basic realm="%s", error="%s", error_description="%s"`,
							s.realm,
							oauthErrorInvalidToken,
							"Api key is invalid",
						),
					)
					c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusUnauthorized,
						Message: "User is not authorized",
					})))
					return
				}
				if !token.HasScope(keyscope, scope) {
					c.SetHeader(
						"WWW-Authenticate",
						fmt.Sprintf(
							`Basic realm="%s", scope="%s", error="%s", error_description="%s"`,
							s.realm,
							scope,
							oauthErrorInsufficientScope,
							"Api key lacks required scope",
						),
					)
					c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "User is forbidden",
					})))
					return
				}
				if !v(s.intersector(userid, keyscope, c)) {
					c.SetHeader(
						"WWW-Authenticate",
						fmt.Sprintf(
							`Basic realm="%s", error="%s", error_description="%s"`,
							s.realm,
							oauthErrorInsufficientScope,
							"User lacks required permission",
						),
					)
					c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "User is forbidden",
					})))
					return
				}
				setCtxUserid(c, userid)
			} else {
				accessToken, err := getAuthHeader(c)
				isBearer := true
				if err != nil {
					if !errors.Is(err, errAuthNotFound{}) {
						c.SetHeader(
							"WWW-Authenticate",
							fmt.Sprintf(
								`Bearer realm="%s", error="%s", error_description="%s"`,
								s.realm,
								oauthErrorInvalidToken,
								"Access token is invalid",
							),
						)
						c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
							Status:  http.StatusUnauthorized,
							Message: "User is not authorized",
						})))
						return
					}
					isBearer = false
					var err error
					accessToken, err = getAccessCookie(r)
					if err != nil {
						c.SetHeader("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, s.realm))
						c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
							Status:  http.StatusUnauthorized,
							Message: "User is not authorized",
						})))
						return
					}
				}
				validToken, claims := s.tokenizer.Validate(token.KindAccess, accessToken)
				if !validToken {
					if isBearer {
						c.SetHeader(
							"WWW-Authenticate",
							fmt.Sprintf(
								`Bearer realm="%s", error="%s", error_description="%s"`,
								s.realm,
								oauthErrorInvalidToken,
								"Access token is invalid",
							),
						)
					}
					c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusUnauthorized,
						Message: "User is not authorized",
					})))
					return
				}
				if !token.HasScope(claims.Scope, scope) {
					if isBearer {
						c.SetHeader(
							"WWW-Authenticate",
							fmt.Sprintf(
								`Bearer realm="%s", scope="%s", error="%s", error_description="%s"`,
								s.realm,
								scope,
								oauthErrorInsufficientScope,
								"Access token lacks required scope",
							),
						)
					}
					c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "User is forbidden",
					})))
					return
				}
				if !v(s.intersector(claims.Subject, claims.Scope, c)) {
					if isBearer {
						c.SetHeader(
							"WWW-Authenticate",
							fmt.Sprintf(
								`Bearer realm="%s", error="%s", error_description="%s"`,
								s.realm,
								oauthErrorInsufficientScope,
								"User lacks required permission",
							),
						)
					}
					c.WriteError(governor.NewError(governor.ErrOptUser, governor.ErrOptRes(governor.ErrorRes{
						Status:  http.StatusForbidden,
						Message: "User is forbidden",
					})))
					return
				}
				setCtxUserid(c, claims.Subject)
				setCtxClaims(c, claims)
			}
			next.ServeHTTP(c.R())
		})
	}
}

// Owner is a middleware function to validate if a user owns the resource
//
// idfunc should return true if the resource is owned by the given user
func Owner(g Gate, idfunc func(governor.Context, string) bool, scope string) governor.Middleware {
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
		return idfunc(r.Ctx(), r.Userid())
	}, scope)
}

// OwnerParam is a middleware function to validate if a url param is the given
// userid
func OwnerParam(g Gate, idparam string, scope string) governor.Middleware {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return Owner(g, func(c governor.Context, userid string) bool {
		return c.Param(idparam) == userid
	}, scope)
}

// Admin is a middleware function to validate if a user is an admin
func Admin(g Gate, scope string) governor.Middleware {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin}))
		if !ok {
			return false
		}
		return roles.Has(rank.TagAdmin)
	}, scope)
}

// User is a middleware function to validate if a user is authenticated and not
// banned
func User(g Gate, scope string) governor.Middleware {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}))
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		return roles.Has(rank.TagUser)
	}, scope)
}

// OwnerOrAdmin is a middleware function to validate if the request is made by
// the resource owner or an admin
//
// idfunc should return true if the resource is owned by the given user
func OwnerOrAdmin(g Gate, idfunc func(governor.Context, string) bool, scope string) governor.Middleware {
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
		return idfunc(r.Ctx(), r.Userid())
	}, scope)
}

// OwnerOrAdminParam is a middleware function to validate if a url param is the
// given userid or if the user is an admin
func OwnerOrAdminParam(g Gate, idparam string, scope string) governor.Middleware {
	if idparam == "" {
		panic("idparam cannot be empty")
	}

	return OwnerOrAdmin(g, func(c governor.Context, userid string) bool {
		return c.Param(idparam) == userid
	}, scope)
}

// ModF is a middleware function to validate if the request is made by the
// moderator of a group or an admin
//
// idfunc should return the group of the resource
func ModF(g Gate, idfunc func(governor.Context, string) (string, error), scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		modtag, err := idfunc(r.Ctx(), r.Userid())
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
	}, scope)
}

// Mod is a middleware function to validate if the request is made by a
// moderator of the group or an admin
func Mod(g Gate, group string, scope string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return ModF(g, func(_ governor.Context, _ string) (string, error) {
		return group, nil
	}, scope)
}

// NoBanF is a middleware function to validate if the request is made by a user
// not banned from the group
//
// idfunc should return the group of the resource
func NoBanF(g Gate, idfunc func(governor.Context, string) (string, error), scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		bantag, err := idfunc(r.Ctx(), r.Userid())
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
	}, scope)
}

// NoBan is a middleware function to validate if the request is made by a
// user not banned from the group
func NoBan(g Gate, group string, scope string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return NoBanF(g, func(_ governor.Context, _ string) (string, error) {
		return group, nil
	}, scope)
}

// MemberF is a middleware function to validate if the request is made by a
// member of a group
//
// idfunc should return the group of the resource
func MemberF(g Gate, idfunc func(governor.Context, string) (string, error), scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(r Intersector) bool {
		tag, err := idfunc(r.Ctx(), r.Userid())
		if err != nil {
			return false
		}
		s := rank.FromSlice([]string{rank.TagAdmin, rank.TagUser})
		if tag != "" {
			s = s.AddUsr(tag).AddBan(tag)
		}
		roles, ok := r.Intersect(s)
		if !ok {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		if tag == "" {
			return true
		}
		return roles.HasUser(tag) && !roles.HasBan(tag)
	}, scope)
}

// Member is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
func Member(g Gate, group string, scope string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return MemberF(g, func(_ governor.Context, _ string) (string, error) {
		return group, nil
	}, scope)
}

// System is a middleware function to validate if the request is made by a system
func System(g Gate, scope string) governor.Middleware {
	return g.Authenticate(func(r Intersector) bool {
		roles, ok := r.Intersect(rank.System())
		if !ok {
			return false
		}
		return roles.Has(rank.TagSystem)
	}, scope)
}
