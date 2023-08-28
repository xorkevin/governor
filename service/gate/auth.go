package gate

import (
	"errors"
	"net/http"
	"strings"

	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/gate/apikey"
	"xorkevin.dev/kerrors"
	"xorkevin.dev/klog"
)

const (
	// KeyIDSystem is the system id key
	KeyIDSystem = "gov.system"
)

const (
	// CookieNameAccessToken is the name of the access token cookie
	CookieNameAccessToken = "gov_access_token"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "gov.all"
	// ScopeNone denies all access
	ScopeNone = "gov.none"
)

// HasScope returns if a token scope contains a scope
func HasScope(tokenScope string, scope string) bool {
	if scope == "" {
		return true
	}
	if scope == ScopeNone {
		return false
	}
	for _, i := range strings.Fields(tokenScope) {
		if i == ScopeAll || i == scope {
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
	}

	// Authorizer is a function to check the authorization of a user
	Authorizer func(c Context) error
)

func (c *Context) HasScope(scope string) bool {
	return HasScope(c.Scope, scope)
}

type (
	ctxKeyUserid struct{}
	ctxKeyClaims struct{}
)

// GetCtxUserid returns a userid from the context
func GetCtxUserid(c *governor.Context) string {
	v := c.Get(ctxKeyUserid{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxApikey(c *governor.Context, userid string, keyid string) {
	c.Set(ctxKeyUserid{}, userid)
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
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authorized"))
					return
				}
				var err error
				token, err = getAccessCookie(c)
				if err != nil {
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authorized"))
					return
				}
			}
			if apitoken, ok := strings.CutPrefix(token, "ga."); ok {
				keyid, keysecret, ok := strings.Cut(apitoken, ".")
				if !ok {
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authorized"))
					return
				}
				userid, keyscope, err := g.CheckKey(c.Ctx(), keyid, keysecret)
				if err != nil {
					if !errors.Is(err, apikey.ErrInvalidKey) && !errors.Is(err, apikey.ErrNotFound) {
						c.WriteError(kerrors.WithMsg(err, "Failed to get apikey"))
						return
					}
					c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authorized"))
					return
				}
				if !HasScope(keyscope, scope) {
					c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
					return
				}
				if err := v(Context{
					Ctx:      c,
					IsSystem: false,
					Userid:   userid,
					Scope:    keyscope,
				}); err != nil {
					c.WriteError(governor.ErrWithRes(err, http.StatusForbidden, "", "User is forbidden"))
					return
				}
				setCtxApikey(c, userid, keyid)
				next.ServeHTTPCtx(c)
				return
			}
			claims, err := g.Validate(c.Ctx(), KindAccess, token)
			if err != nil {
				c.WriteError(governor.ErrWithRes(err, http.StatusUnauthorized, "", "User is not authorized"))
				return
			}
			if !HasScope(claims.Scope, scope) {
				c.WriteError(governor.ErrWithRes(nil, http.StatusForbidden, "", "User is forbidden"))
				return
			}
			if err := v(Context{
				Ctx:      c,
				IsSystem: claims.Subject == KeyIDSystem,
				Userid:   claims.Subject,
				Scope:    claims.Scope,
			}); err != nil {
				c.WriteError(governor.ErrWithRes(err, http.StatusForbidden, "", "User is forbidden"))
				return
			}
			setCtxClaims(c, claims)
			next.ServeHTTPCtx(c)
		})
	}
}
