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
	"xorkevin.dev/kerrors"
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
		Authenticate(v Authorizer, scope string) governor.Middleware
		Authorize(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error)
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

	// Intersector returns roles needed to validate a user
	Intersector interface {
		Intersect(ctx context.Context, roles rank.Rank) (rank.Rank, error)
	}

	// Context holds an auth context
	Context interface {
		Intersector
		Userid() string
		HasScope(scope string) bool
		Ctx() governor.Context
	}

	authctx struct {
		s      *service
		userid string
		scope  string
		ctx    governor.Context
	}

	// Authorizer is a function to check the authorization of a user
	Authorizer func(c Context) bool

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

func (s *service) Register(name string, inj governor.Injector, r governor.ConfigRegistrar, jr governor.JobRegistrar) {
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
	l.Info("Loaded config", map[string]string{
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

func (r *authctx) Userid() string {
	return r.userid
}

func (r *authctx) Ctx() governor.Context {
	return r.ctx
}

func (r *authctx) Intersect(ctx context.Context, roles rank.Rank) (rank.Rank, error) {
	k, err := r.s.Authorize(ctx, r.userid, roles)
	if err != nil {
		r.s.logger.Error("Failed to get user roles", map[string]string{
			"error":      err.Error(),
			"actiontype": "authgetroles",
		})
		return nil, err
	}
	return k, nil
}

func (r *authctx) HasScope(scope string) bool {
	return token.HasScope(r.scope, scope)
}

func (s *service) authctx(userid string, scope string, ctx governor.Context) Context {
	return &authctx{
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
func (s *service) Authenticate(v Authorizer, scope string) governor.Middleware {
	if v == nil {
		panic("authorizer cannot be nil")
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c := governor.NewContext(w, r, s.logger)
			keyid, password, ok := c.BasicAuth()
			if ok {
				userid, keyscope, err := s.apikeys.CheckKey(c.Ctx(), keyid, password)
				if err != nil {
					if !errors.Is(err, apikey.ErrInvalidKey{}) && !errors.Is(err, apikey.ErrNotFound{}) {
						c.WriteError(kerrors.WithMsg(err, "Failed to get apikey"))
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
					c.WriteError(governor.ErrWithRes(nil, http.StatusUnauthorized, "", "User is not authorized"))
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
					c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
					return
				}
				if !v(s.authctx(userid, keyscope, c)) {
					c.SetHeader(
						"WWW-Authenticate",
						fmt.Sprintf(
							`Basic realm="%s", error="%s", error_description="%s"`,
							s.realm,
							oauthErrorInsufficientScope,
							"User lacks required permission",
						),
					)
					c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
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
						c.WriteError(governor.ErrWithRes(nil, http.StatusUnauthorized, "", "User is not authorized"))
						return
					}
					isBearer = false
					var err error
					accessToken, err = getAccessCookie(r)
					if err != nil {
						c.SetHeader("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s"`, s.realm))
						c.WriteError(governor.ErrWithRes(nil, http.StatusUnauthorized, "", "User is not authorized"))
						return
					}
				}
				validToken, claims := s.tokenizer.Validate(c.Ctx(), token.KindAccess, accessToken)
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
					c.WriteError(governor.ErrWithRes(nil, http.StatusUnauthorized, "", "User is not authorized"))
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
					c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
					return
				}
				if !v(s.authctx(claims.Subject, claims.Scope, c)) {
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
					c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
					return
				}
				setCtxUserid(c, claims.Subject)
				setCtxClaims(c, claims)
			}
			next.ServeHTTP(c.R())
		})
	}
}

// Authorize authorizes a user for some given roles
func (s *service) Authorize(ctx context.Context, userid string, roles rank.Rank) (rank.Rank, error) {
	return s.roles.IntersectRoles(ctx, userid, roles)
}

func checkErrBool(b bool, err error) bool {
	if err != nil {
		return false
	}
	return b
}

// wrapIntersector wraps an intersector as an Authorizer
func wrapIntersector(f func(ctx context.Context, r Intersector) (bool, error)) Authorizer {
	if f == nil {
		panic("intersector cannot be nil")
	}

	return func(c Context) bool {
		return checkErrBool(f(c.Ctx().Ctx(), c))
	}
}

type (
	authint struct {
		g      Gate
		userid string
	}
)

func (r *authint) Intersect(ctx context.Context, roles rank.Rank) (rank.Rank, error) {
	return r.g.Authorize(ctx, r.userid, roles)
}

func newIntersector(g Gate, userid string) Intersector {
	return &authint{
		g:      g,
		userid: userid,
	}
}

// Owner is a middleware function to validate if a user owns the resource
//
// idfunc should return true if the resource is owned by the given user
func Owner(g Gate, idfunc func(governor.Context, string) bool, scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(c Context) bool {
		roles, err := c.Intersect(c.Ctx().Ctx(), rank.FromSlice([]string{rank.TagUser}))
		if err != nil {
			return false
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return idfunc(c.Ctx(), c.Userid())
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

func checkAdmin(ctx context.Context, r Intersector) (bool, error) {
	roles, err := r.Intersect(ctx, rank.FromSlice([]string{rank.TagAdmin}))
	if err != nil {
		return false, err
	}
	return roles.Has(rank.TagAdmin), nil
}

// AuthAdmin authorizes a user as an admin
func AuthAdmin(ctx context.Context, g Gate, userid string) (bool, error) {
	return checkAdmin(ctx, newIntersector(g, userid))
}

// Admin is a middleware function to validate if a user is an admin
func Admin(g Gate, scope string) governor.Middleware {
	return g.Authenticate(wrapIntersector(checkAdmin), scope)
}

func checkUser(ctx context.Context, r Intersector) (bool, error) {
	roles, err := r.Intersect(ctx, rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}))
	if err != nil {
		return false, err
	}
	if roles.Has(rank.TagAdmin) {
		return true, nil
	}
	return roles.Has(rank.TagUser), nil
}

// AuthUser authorizes a user as not banned
func AuthUser(ctx context.Context, g Gate, userid string) (bool, error) {
	return checkUser(ctx, newIntersector(g, userid))
}

// User is a middleware function to validate if a user is authenticated and not
// banned
func User(g Gate, scope string) governor.Middleware {
	return g.Authenticate(wrapIntersector(checkUser), scope)
}

// OwnerOrAdmin is a middleware function to validate if the request is made by
// the resource owner or an admin
//
// idfunc should return true if the resource is owned by the given user
func OwnerOrAdmin(g Gate, idfunc func(governor.Context, string) bool, scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(c Context) bool {
		roles, err := c.Intersect(c.Ctx().Ctx(), rank.FromSlice([]string{rank.TagAdmin, rank.TagUser}))
		if err != nil {
			return false
		}
		if roles.Has(rank.TagAdmin) {
			return true
		}
		if !roles.Has(rank.TagUser) {
			return false
		}
		return idfunc(c.Ctx(), c.Userid())
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

func checkMod(ctx context.Context, r Intersector, modtag string, isSelf bool) (bool, error) {
	roleQuery := rank.FromSlice([]string{rank.TagAdmin, rank.TagUser})
	if modtag != "" {
		roleQuery.AddMod(modtag)
	}
	roles, err := r.Intersect(ctx, roleQuery)
	if err != nil {
		return false, err
	}
	if roles.Has(rank.TagAdmin) {
		return true, nil
	}
	if !roles.Has(rank.TagUser) {
		return false, nil
	}
	if isSelf {
		return true, nil
	}
	if modtag == "" {
		return false, nil
	}
	return roles.HasMod(modtag), nil
}

// AuthMod authorizes a user as a mod
func AuthMod(ctx context.Context, g Gate, userid string, modtag string) (bool, error) {
	return checkMod(ctx, newIntersector(g, userid), modtag, false)
}

// ModF is a middleware function to validate if the request is made by the
// moderator of a group or an admin
//
// idfunc should return the group of the resource
func ModF(g Gate, idfunc func(governor.Context, string) (string, bool, bool), scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(c Context) bool {
		modtag, isSelf, ok := idfunc(c.Ctx(), c.Userid())
		if !ok {
			return false
		}
		return checkErrBool(checkMod(c.Ctx().Ctx(), c, modtag, isSelf))
	}, scope)
}

// Mod is a middleware function to validate if the request is made by a
// moderator of the group or an admin
func Mod(g Gate, group string, scope string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return ModF(g, func(_ governor.Context, _ string) (string, bool, bool) {
		return group, false, true
	}, scope)
}

func checkNoBan(ctx context.Context, r Intersector, bantag string, isSelf bool) (bool, error) {
	roleQuery := rank.FromSlice([]string{rank.TagAdmin, rank.TagUser})
	if bantag != "" {
		roleQuery.AddBan(bantag)
	}
	roles, err := r.Intersect(ctx, roleQuery)
	if err != nil {
		return false, err
	}
	if roles.Has(rank.TagAdmin) {
		return true, nil
	}
	if !roles.Has(rank.TagUser) {
		return false, nil
	}
	if isSelf {
		return true, nil
	}
	if bantag == "" {
		return false, nil
	}
	return !roles.HasBan(bantag), nil
}

// AuthNoBan authorizes a user as not banned
func AuthNoBan(ctx context.Context, g Gate, userid string, bantag string) (bool, error) {
	return checkNoBan(ctx, newIntersector(g, userid), bantag, false)
}

// NoBanF is a middleware function to validate if the request is made by a user
// not banned from the group
//
// idfunc should return the group of the resource
func NoBanF(g Gate, idfunc func(governor.Context, string) (string, bool, bool), scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(c Context) bool {
		bantag, isSelf, ok := idfunc(c.Ctx(), c.Userid())
		if !ok {
			return false
		}
		return checkErrBool(checkNoBan(c.Ctx().Ctx(), c, bantag, isSelf))
	}, scope)
}

// NoBan is a middleware function to validate if the request is made by a
// user not banned from the group
func NoBan(g Gate, group string, scope string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return NoBanF(g, func(_ governor.Context, _ string) (string, bool, bool) {
		return group, false, true
	}, scope)
}

func checkMember(ctx context.Context, r Intersector, tag string, isSelf bool) (bool, error) {
	roleQuery := rank.FromSlice([]string{rank.TagAdmin, rank.TagUser})
	if tag != "" {
		roleQuery.AddUsr(tag).AddBan(tag)
	}
	roles, err := r.Intersect(ctx, roleQuery)
	if err != nil {
		return false, err
	}
	if roles.Has(rank.TagAdmin) {
		return true, nil
	}
	if !roles.Has(rank.TagUser) {
		return false, nil
	}
	if isSelf {
		return true, nil
	}
	if tag == "" {
		return false, nil
	}
	return roles.HasUser(tag) && !roles.HasBan(tag), nil
}

// AuthMember authorizes a user as a group member
func AuthMember(ctx context.Context, g Gate, userid string, tag string) (bool, error) {
	return checkMember(ctx, newIntersector(g, userid), tag, false)
}

// MemberF is a middleware function to validate if the request is made by a
// member of a group
//
// idfunc should return the group of the resource and whether the resource is owned by self
// allowSelf allows the self group
func MemberF(g Gate, idfunc func(governor.Context, string) (string, bool, bool), scope string) governor.Middleware {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}

	return g.Authenticate(func(c Context) bool {
		tag, isSelf, ok := idfunc(c.Ctx(), c.Userid())
		if !ok {
			return false
		}
		return checkErrBool(checkMember(c.Ctx().Ctx(), c, tag, isSelf))
	}, scope)
}

// Member is a middleware function to validate if the request is made by a
// member of a group and check if the user is banned from the group
func Member(g Gate, group string, scope string) governor.Middleware {
	if group == "" {
		panic("group cannot be empty")
	}

	return MemberF(g, func(_ governor.Context, _ string) (string, bool, bool) {
		return group, false, true
	}, scope)
}

func checkSystem(ctx context.Context, r Intersector) (bool, error) {
	roles, err := r.Intersect(ctx, rank.System())
	if err != nil {
		return false, err
	}
	return roles.Has(rank.TagSystem), nil
}

func AuthSystem(ctx context.Context, g Gate, userid string) (bool, error) {
	return checkSystem(ctx, newIntersector(g, userid))
}

// System is a middleware function to validate if the request is made by a system
func System(g Gate, scope string) governor.Middleware {
	return g.Authenticate(wrapIntersector(checkSystem), scope)
}
