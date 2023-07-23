package gate

import (
	"time"

	"github.com/go-jose/go-jose/v3"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/util/ksync"
	"xorkevin.dev/governor/util/lifecycle"
	"xorkevin.dev/hunter2/h2signer"
	"xorkevin.dev/hunter2/h2signer/eddsa"
	"xorkevin.dev/hunter2/h2signer/rs256"
	"xorkevin.dev/klog"
)

const (
	// CookieNameAccessToken is the name of the access token cookie
	CookieNameAccessToken = "access_token"
)

const (
	// ScopeAll grants all scopes to a token
	ScopeAll = "all"
	// ScopeForbidden denies all access
	ScopeForbidden = "forbidden"
)

type (
	// Kind is a token kind
	Kind string
)

const (
	// KindAccess is an access token kind
	KindAccess Kind = "access"
	// KindRefresh is a refresh token kind
	KindRefresh = "refresh"
	// KindSystem is a system token kind
	KindSystem = "system"
	// KindOAuthAccess is an oauth access token kind
	KindOAuthAccess = "oauth:access"
	// KindOAuthRefresh is an oauth refresh token kind
	KindOAuthRefresh = "oauth:refresh"
	// KindOAuthID is an openid id token kind
	KindOAuthID = "oauth:id"
)

const (
	jwtHeaderKid = "kid"
	jwtHeaderJWT = "JWT"
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
		Kind      string `json:"kind,omitempty"`
		SessionID string `json:"sess,omitempty"`
		AuthTime  int64  `json:"aat,omitempty"`
		Scope     string `json:"scope,omitempty"`
		Key       string `json:"key,omitempty"`
	}

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

type (
	Gate interface{}

	tokenSigner struct {
		signer          jose.Signer
		extsigner       jose.Signer
		signingkeys     *h2signer.SigningKeyring
		extsigningkeys  *h2signer.SigningKeyring
		sysverifierkeys *h2signer.VerifierKeyring
		jwks            []jose.JSONWebKey
		eddsaid         string
		rs256id         string
	}

	Service struct {
		lc           *lifecycle.Lifecycle[tokenSigner]
		issuer       string
		audience     string
		signingAlgs  h2signer.SigningKeyAlgs
		verifierAlgs h2signer.VerifierKeyAlgs
		config       governor.SecretReader
		log          *klog.LevelLogger
		hbfailed     int
		hbmaxfail    int
		keyrefresh   time.Duration
		wg           *ksync.WaitGroup
	}

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

// New creates a new Tokenizer
func New() *Service {
	signingAlgs := h2signer.NewSigningKeysMap()
	eddsa.RegisterSigner(signingAlgs)
	rs256.Register(signingAlgs)
	verifierAlgs := h2signer.NewVerifierKeysMap()
	eddsa.RegisterVerifier(verifierAlgs)
	return &Service{
		signingAlgs:  signingAlgs,
		verifierAlgs: verifierAlgs,
		hbfailed:     0,
		wg:           ksync.NewWaitGroup(),
	}
}
