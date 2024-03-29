package gate

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/gate/apikey"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	// KeySubSystem is the system id key
	KeySubSystem = "gov.system"
)

const (
	// CookieNameAccessToken is the name of the access token cookie
	CookieNameAccessToken = "gov_access"
)

const (
	// ScopeAll allows all access
	ScopeAll = "gov.all"
	// ScopeNone denies all access
	ScopeNone = "gov.none"
)

// HasScope returns if a token scope contains a scope
func HasScope(tokenScope string, scope string) bool {
	if tokenScope == ScopeAll {
		return true
	}
	if scope == "" {
		return true
	}
	if scope == ScopeNone {
		return false
	}
	for _, i := range strings.Fields(tokenScope) {
		if i == scope {
			return true
		}
	}
	return false
}

type (
	Context struct {
		Ctx      *governor.Context
		IsSystem bool
		Userid   string
		Scope    string
		AuthAt   time.Time
		IsApikey bool
		ApikeyID string
	}

	// Authorizer is a function to check the authorization of a user
	Authorizer func(c Context, acl ACL) (bool, error)
)

func (c *Context) HasScope(scope string) bool {
	return HasScope(c.Scope, scope)
}

type (
	ctxKeyUserid   struct{}
	ctxKeyApikeyID struct{}
	ctxKeyClaims   struct{}
)

// GetCtxUserid returns a userid from the context
func GetCtxUserid(c *governor.Context) string {
	v := c.Get(ctxKeyUserid{})
	if v == nil {
		return ""
	}
	return v.(string)
}

// GetCtxApikeyID returns an apikeyid from the context
func GetCtxApikeyID(c *governor.Context) string {
	v := c.Get(ctxKeyApikeyID{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxApikey(c *governor.Context, userid string, keyid string) {
	c.Set(ctxKeyUserid{}, userid)
	c.Set(ctxKeyApikeyID{}, keyid)
	c.LogAttrs(klog.AGroup("gate",
		klog.AString("userid", userid),
		klog.AString("keyid", keyid),
	))
}

// GetCtxClaims returns token claims from the context
func GetCtxClaims(c *governor.Context) *Claims {
	v := c.Get(ctxKeyClaims{})
	if v == nil {
		return nil
	}
	return v.(*Claims)
}

func setCtxClaims(c *governor.Context, claims *Claims) {
	c.Set(ctxKeyUserid{}, claims.Subject)
	c.Set(ctxKeyClaims{}, claims)
	c.LogAttrs(klog.AGroup("gate",
		klog.AString("userid", claims.Subject),
		klog.AString("sessionid", claims.SessionID),
		klog.AString("tokenid", claims.ID),
	))
}

var (
	// ErrAuthNotFound is returned when an auth header is not found
	ErrAuthNotFound errAuthNotFound
	// ErrInvalidHeader is returned when an auth header is malformed
	ErrInvalidHeader errInvalidHeader
)

type (
	errAuthNotFound  struct{}
	errInvalidHeader struct{}
)

func (e errAuthNotFound) Error() string {
	return "Auth not found"
}

func (e errInvalidHeader) Error() string {
	return "Invalid auth header"
}

func getAuthHeader(c *governor.Context) (string, error) {
	authHeader := c.Header("Authorization")
	if authHeader == "" {
		return "", kerrors.WithKind(nil, ErrAuthNotFound, "Missing auth header")
	}
	scheme, token, ok := strings.Cut(authHeader, " ")
	if !ok || scheme != "Bearer" || token == "" {
		return "", kerrors.WithKind(nil, ErrInvalidHeader, "Invalid auth header")
	}
	return token, nil
}

func getAccessCookie(c *governor.Context) (string, error) {
	cookie, err := c.Cookie(CookieNameAccessToken)
	if err != nil {
		return "", err
	}
	if cookie.Value == "" {
		return "", ErrAuthNotFound
	}
	return cookie.Value, nil
}

func AuthenticateCtx(g Gate, v Authorizer, scope string) governor.MiddlewareCtx {
	return func(next governor.RouteHandler) governor.RouteHandler {
		return governor.RouteHandlerFunc(func(c *governor.Context) {
			token, err := getAuthHeader(c)
			if err != nil {
				if !errors.Is(err, ErrAuthNotFound) {
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authenticated"))
					return
				}
				var err error
				token, err = getAccessCookie(c)
				if err != nil {
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authenticated"))
					return
				}
			}
			var ctx Context
			if apitoken, ok := strings.CutPrefix(token, "ga."); ok {
				keyid, keysecret, ok := strings.Cut(apitoken, ".")
				if !ok {
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid api token"))
					return
				}
				userscope, err := g.CheckKey(c.Ctx(), keyid, keysecret)
				if err != nil {
					if !errors.Is(err, apikey.ErrInvalidKey) && !errors.Is(err, apikey.ErrNotFound) {
						c.WriteError(kerrors.WithMsg(err, "Failed to get apikey"))
						return
					}
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid api token"))
					return
				}
				ctx = Context{
					Ctx:      c,
					IsSystem: false,
					Userid:   userscope.Userid,
					Scope:    userscope.Scope,
					IsApikey: true,
					ApikeyID: keyid,
				}
				setCtxApikey(c, userscope.Userid, keyid)
			} else {
				claims, err := g.Validate(c.Ctx(), token)
				if err != nil {
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "Invalid access token"))
					return
				}
				ctx = Context{
					Ctx:      c,
					IsSystem: claims.Subject == KeySubSystem,
					Userid:   claims.Subject,
					Scope:    claims.Scope,
					AuthAt:   time.Unix(claims.AuthAt, 0).UTC(),
					IsApikey: false,
				}
				if claims.Kind == "" {
					ctx.Scope = ScopeAll
				}
				setCtxClaims(c, claims)
			}
			if !ctx.HasScope(scope) {
				c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "Insufficient scope"))
				return
			}
			if ok, err := v(ctx, g); err != nil {
				c.WriteError(kerrors.WithMsg(err, "Authorization error"))
				return
			} else if !ok {
				c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
			}
			next.ServeHTTPCtx(c)
		})
	}
}

const (
	NSUser    = "gov.user"
	RelIn     = "in"
	RelMod    = "mod"
	NSRole    = "gov.role"
	RoleUser  = "gov.user"
	RoleAdmin = "gov.admin"
)

func CheckRole(ctx context.Context, acl authzacl.ACL, userid string, role string) (bool, error) {
	return acl.Check(ctx, authzacl.Obj{
		NS:  NSRole,
		Key: role,
	}, RelIn, authzacl.Sub{
		NS:  NSUser,
		Key: userid,
	})
}

func CheckMod(ctx context.Context, acl authzacl.ACL, userid string, role string) (bool, error) {
	return acl.Check(ctx, authzacl.Obj{
		NS:  NSRole,
		Key: role,
	}, RelMod, authzacl.Sub{
		NS:  NSUser,
		Key: userid,
	})
}

func checkRole(c Context, acl ACL, role string) (bool, error) {
	return acl.CheckRel(c.Ctx.Ctx(), authzacl.Obj{
		NS:  NSRole,
		Key: role,
	}, RelIn, authzacl.Sub{
		NS:  NSUser,
		Key: c.Userid,
	})
}

// AuthSystem is a middleware function to validate a sys token
func AuthSystem(g Gate, scope string) governor.MiddlewareCtx {
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		return c.IsSystem, nil
	}, scope)
}

// AuthUser is a middleware function to validate if a user is authenticated
func AuthUser(g Gate, scope string) governor.MiddlewareCtx {
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		if ok, err := checkRole(c, acl, RoleAdmin); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
		return checkRole(c, acl, RoleUser)
	}, scope)
}

// AuthAdmin is a middleware function to validate if a user is an admin
func AuthAdmin(g Gate, scope string) governor.MiddlewareCtx {
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		return checkRole(c, acl, RoleAdmin)
	}, scope)
}

// AuthUserSudo is a middleware function to validate if a user logged in
// recently
func AuthUserSudo(g Gate, d time.Duration, scope string) governor.MiddlewareCtx {
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		if !c.IsApikey && time.Now().Round(0).After(c.AuthAt.Add(d)) {
			return false, governor.ErrWithRes(nil, http.StatusForbidden, "", "Must auth again")
		}
		if ok, err := checkRole(c, acl, RoleAdmin); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
		return checkRole(c, acl, RoleUser)
	}, scope)
}

// AuthAdminSudo is a middleware function to validate if a user is an admin and
// logged in recently
func AuthAdminSudo(g Gate, d time.Duration, scope string) governor.MiddlewareCtx {
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		if !c.IsApikey && time.Now().Round(0).After(c.AuthAt.Add(d)) {
			return false, governor.ErrWithRes(nil, http.StatusForbidden, "", "Must auth again")
		}
		return checkRole(c, acl, RoleAdmin)
	}, scope)
}

// AuthOwner is a middleware function to validate if a user owns the resource
//
// idfunc should return true if the resource is owned by the given user
func AuthOwner(g Gate, idfunc func(Context) (bool, error), scope string) governor.MiddlewareCtx {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		if ok, err := checkRole(c, acl, RoleUser); err != nil {
			return false, err
		} else if !ok {
			return false, nil
		}
		return idfunc(c)
	}, scope)
}

// AuthOwnerParam is a middleware function to validate if a url param is the
// given userid
func AuthOwnerParam(g Gate, idparam string, scope string) governor.MiddlewareCtx {
	if idparam == "" {
		panic("idparam cannot be empty")
	}
	return AuthOwner(g, func(c Context) (bool, error) {
		return c.Ctx.Param(idparam) == c.Userid, nil
	}, scope)
}

// AuthOwnerOrAdmin is a middleware function to validate if a user is the
// resource owner or an admin
//
// idfunc should return true if the resource is owned by the given user
func AuthOwnerOrAdmin(g Gate, idfunc func(Context) (bool, error), scope string) governor.MiddlewareCtx {
	if idfunc == nil {
		panic("idfunc cannot be nil")
	}
	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		if ok, err := checkRole(c, acl, RoleAdmin); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
		if ok, err := checkRole(c, acl, RoleUser); err != nil {
			return false, err
		} else if !ok {
			return false, nil
		}
		return idfunc(c)
	}, scope)
}

// AuthOwnerOrAdminParam is a middleware function to validate if a url param is
// the given userid or if the user is an admin
func AuthOwnerOrAdminParam(g Gate, idparam string, scope string) governor.MiddlewareCtx {
	if idparam == "" {
		panic("idparam cannot be empty")
	}
	return AuthOwnerOrAdmin(g, func(c Context) (bool, error) {
		return c.Ctx.Param(idparam) == c.Userid, nil
	}, scope)
}

// AuthMember is a middleware function to validate if a user has a role
func AuthMember(g Gate, role string, scope string) governor.MiddlewareCtx {
	if role == "" {
		panic("role cannot be empty")
	}

	return AuthenticateCtx(g, func(c Context, acl ACL) (bool, error) {
		if ok, err := checkRole(c, acl, RoleAdmin); err != nil {
			return false, err
		} else if ok {
			return true, nil
		}
		if ok, err := checkRole(c, acl, RoleUser); err != nil {
			return false, err
		} else if !ok {
			return false, nil
		}
		return checkRole(c, acl, role)
	}, scope)
}
