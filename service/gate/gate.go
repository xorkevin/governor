package gate

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/klog"
)

const (
	// CookieNameAccessToken is the name of the access token cookie
	CookieNameAccessToken = "access_token"
)

type (
	Claims struct {
		Issuer    string   `json:"iss,omitempty"`
		Subject   string   `json:"sub,omitempty"`
		Audience  []string `json:"aud,omitempty"`
		Expiry    int64    `json:"exp,omitempty"`
		NotBefore int64    `json:"nbf,omitempty"`
		IssuedAt  int64    `json:"iat,omitempty"`
		ID        string   `json:"jti,omitempty"`
		// Custom fields
		Kind     string `json:"kind,omitempty"`
		AuthTime int64  `json:"aat,omitempty"`
		Scope    string `json:"scope,omitempty"`
		Key      string `json:"key,omitempty"`
	}

	ctxKeyUserid    struct{}
	ctxKeyClaims    struct{}
	ctxKeySysUserid struct{}
)

// GetCtxUserid returns a userid from the context
func GetCtxUserid(c *governor.Context) string {
	v := c.Get(ctxKeyUserid{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxUserid(c *governor.Context, userid string) {
	c.Set(ctxKeyUserid{}, userid)
	c.LogAttrs(klog.AString("gate.userid", userid))
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
	c.LogAttrs(
		klog.AString("gate.userid", claims.Subject),
		klog.AString("gate.sessionid", claims.ID),
	)
}

// GetCtxSysUserid returns a system userid from the context
func GetCtxSysUserid(c *governor.Context) string {
	v := c.Get(ctxKeySysUserid{})
	if v == nil {
		return ""
	}
	return v.(string)
}

func setCtxSystem(c *governor.Context, claims *Claims) {
	c.Set(ctxKeySysUserid{}, claims.Subject)
	c.LogAttrs(
		klog.AString("gate.sysuserid", claims.Subject),
		klog.AString("gate.syssessionid", claims.ID),
	)
}
