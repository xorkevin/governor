package gate

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/go-jose/go-jose/v3"
	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/governortest"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/gate/apikey"
	"xorkevin.dev/hunter2/h2signer/rsasig"
	"xorkevin.dev/kfs/kfstest"
	"xorkevin.dev/klog"
)

type (
	testServiceA struct {
		g Gate
	}
)

func (s *testServiceA) Register(r governor.ConfigRegistrar) {
}

func (s *testServiceA) Init(ctx context.Context, r governor.ConfigReader, kit governor.ServiceKit) error {
	mr := governor.NewMethodRouter(kit.Router)
	mr.GetCtx("/user", func(c *governor.Context) {
		c.WriteStatus(http.StatusOK)
	}, AuthUser(s.g, "test-scope"))
	mr.GetCtx("/admin", func(c *governor.Context) {
		c.WriteStatus(http.StatusOK)
	}, AuthAdmin(s.g, "test-scope"))
	mr.GetCtx("/user/{id}", func(c *governor.Context) {
		c.WriteStatus(http.StatusOK)
	}, AuthOwnerOrAdminParam(s.g, "id", "test-scope"))
	mr.GetCtx("/user/{id}/private", func(c *governor.Context) {
		c.WriteStatus(http.StatusOK)
	}, AuthOwnerParam(s.g, "id", "test-scope"))
	return nil
}

func (s *testServiceA) Start(ctx context.Context) error {
	return nil
}

func (s *testServiceA) Stop(ctx context.Context) {
}

func (s *testServiceA) Setup(ctx context.Context, req governor.ReqSetup) error {
	return nil
}

func (s *testServiceA) Health(ctx context.Context) error {
	return nil
}

func TestGate(t *testing.T) {
	t.Parallel()

	assert := require.New(t)

	term := governortest.NewTestTerm()
	var out bytes.Buffer
	term.Stdout = &out
	fsys := &kfstest.MapFS{
		Fsys: fstest.MapFS{},
	}
	term.Fsys = fsys

	{
		client := governortest.NewTestClient(t, nil, strings.NewReader(`
{
  "gate": {
    "keyfile": "key.txt",
    "tokenfile": "token.txt"
  }
}
`), term)

		gateClient := NewCmdClient()
		client.Register("gate", "/null/gate", &governor.CmdDesc{
			Usage: "gate",
			Short: "gate",
			Long:  "gate",
		}, gateClient)
		assert.NoError(client.Init(governor.ClientFlags{}, klog.Discard{}))

		assert.NoError(gateClient.genKey(nil))
		assert.NotNil(fsys.Fsys["key.txt"])
		gateClient.tokenFlags.subject = KeySubSystem
		gateClient.tokenFlags.expirestr = "1h"
		assert.NoError(gateClient.genToken(nil))
		assert.NotNil(fsys.Fsys["token.txt"])
		token, err := gateClient.GetToken()
		assert.NoError(err)
		assert.Equal(string(bytes.TrimSpace(fsys.Fsys["token.txt"].Data)), token)

		assert.NoError(gateClient.validateToken(nil))
		var claims Claims
		assert.NoError(json.Unmarshal(out.Bytes(), &claims))
		assert.Equal(KeySubSystem, claims.Subject)
		assert.Equal("", claims.Kind)
		assert.Equal("", claims.Scope)
	}

	rsakey, err := rsa.GenerateKey(rand.Reader, 1024)
	assert.NoError(err)
	rsaconfig := rsasig.Config{
		Key: rsakey,
	}
	rsastr, err := rsaconfig.String()
	assert.NoError(err)

	server := governortest.NewTestServer(t, map[string]any{
		"gate": map[string]any{
			"tokensecret": "tokensecret",
			"issuer":      "test-issuer",
		},
	}, map[string]any{
		"data": map[string]any{
			"tokensecret": map[string]any{
				"keys":    []string{string(bytes.TrimSpace(fsys.Fsys["key.txt"].Data))},
				"extkeys": []string{rsastr},
			},
		},
	}, nil)

	acl := authzacl.ACLSet{
		Set: map[authzacl.Relation]struct{}{},
	}
	acl.AddRelations(context.Background(),
		authzacl.Rel(NSRole, RoleUser, RelIn, NSUser, "test-admin-1", ""),
		authzacl.Rel(NSRole, RoleAdmin, RelIn, NSUser, "test-admin-1", ""),
		authzacl.Rel(NSRole, RoleUser, RelIn, NSUser, "test-user-1", ""),
	)

	keyset := apikey.KeySet{
		Set: map[string]apikey.MemKey{},
	}
	akey, err := keyset.InsertKey(context.Background(), "test-user-1", "other-scope test-scope", "", "")
	assert.NoError(err)

	g := New(&acl, &keyset)
	server.Register("gate", "/null/gate", g)
	server.Register("test", "/test", &testServiceA{g: g})

	assert.NoError(server.Start(context.Background(), governor.Flags{}, klog.Discard{}))

	{
		jwks, err := g.GetJWKS(context.Background())
		assert.NoError(err)
		assert.Len(jwks.Keys, 1)
		assert.Equal(string(jose.RS256), jwks.Keys[0].Algorithm)
	}

	usertoken, _, err := g.Generate(context.Background(), Claims{
		Subject:   "test-user-1",
		SessionID: "test-session-id",
		AuthAt:    time.Now().Round(0).Unix(),
	}, 1*time.Minute)
	assert.NoError(err)

	admintoken, _, err := g.Generate(context.Background(), Claims{
		Subject:   "test-admin-1",
		SessionID: "test-session-id",
		AuthAt:    time.Now().Round(0).Unix(),
	}, 1*time.Minute)
	assert.NoError(err)

	{
		claims, err := g.Validate(context.Background(), usertoken)
		assert.NoError(err)
		assert.Equal("test-user-1", claims.Subject)
		assert.Equal("test-session-id", claims.SessionID)
		assert.Equal("", claims.Scope)
		assert.NotEmpty(claims.Expiry)
		assert.NotEmpty(claims.ID)
	}

	{
		token, err := g.GenerateExt(context.Background(), Claims{
			Subject:   "test-user-1",
			SessionID: "test-session-id",
			Audience:  []string{"test-audience"},
			Scope:     "openid profile",
		}, 1*time.Minute, nil)
		assert.NoError(err)

		claims, err := g.ValidateExt(context.Background(), token, nil)
		assert.NoError(err)
		assert.Equal("test-user-1", claims.Subject)
		assert.Equal("test-session-id", claims.SessionID)
		assert.Equal([]string{"test-audience"}, claims.Audience)
		assert.Equal("openid profile", claims.Scope)
		assert.Equal(kindOpenID, claims.Kind)
		assert.Equal("test-issuer", claims.Issuer)
		assert.NotEmpty(claims.Expiry)
		assert.NotEmpty(claims.ID)
	}

	{
		token, err := g.GenerateExt(context.Background(), Claims{
			Subject:   "test-user-1",
			SessionID: "test-session-id",
			Audience:  []string{"test-audience"},
			Scope:     "openid profile",
		}, 1*time.Minute, map[string]any{
			"custom": "value",
		})
		assert.NoError(err)

		var otherClaims struct {
			Custom string `json:"custom"`
		}
		claims, err := g.ValidateExt(context.Background(), token, &otherClaims)
		assert.NoError(err)
		assert.Equal("value", otherClaims.Custom)
		assert.Equal("test-user-1", claims.Subject)
		assert.Equal("test-session-id", claims.SessionID)
		assert.Equal([]string{"test-audience"}, claims.Audience)
		assert.Equal("openid profile", claims.Scope)
		assert.Equal(kindOpenID, claims.Kind)
		assert.Equal("test-issuer", claims.Issuer)
		assert.NotEmpty(claims.Expiry)
		assert.NotEmpty(claims.ID)
	}

	for _, tc := range []struct {
		Name   string
		Path   string
		Token  string
		Status int
	}{
		{
			Name:   "user token success",
			Path:   "/api/test/user",
			Token:  usertoken,
			Status: http.StatusOK,
		},
		{
			Name:   "user token failure",
			Path:   "/api/test/admin",
			Token:  usertoken,
			Status: http.StatusForbidden,
		},
		{
			Name:   "admin token success",
			Path:   "/api/test/admin",
			Token:  admintoken,
			Status: http.StatusOK,
		},
		{
			Name:   "api key success",
			Path:   "/api/test/user",
			Token:  akey.Key,
			Status: http.StatusOK,
		},
		{
			Name:   "api key failure",
			Path:   "/api/test/admin",
			Token:  akey.Key,
			Status: http.StatusForbidden,
		},
		{
			Name:   "user token check owner success",
			Path:   "/api/test/user/test-user-1/private",
			Token:  usertoken,
			Status: http.StatusOK,
		},
		{
			Name:   "user token check owner failure",
			Path:   "/api/test/user/test-user-2/private",
			Token:  usertoken,
			Status: http.StatusForbidden,
		},
		{
			Name:   "admin token check owner success",
			Path:   "/api/test/user/test-user-1",
			Token:  admintoken,
			Status: http.StatusOK,
		},
		{
			Name:   "admin token check owner failure",
			Path:   "/api/test/user/test-user-1/private",
			Token:  admintoken,
			Status: http.StatusForbidden,
		},
		{
			Name:   "api key check owner success",
			Path:   "/api/test/user/test-user-1",
			Token:  akey.Key,
			Status: http.StatusOK,
		},
		{
			Name:   "api key check owner failure",
			Path:   "/api/test/user/test-user-2",
			Token:  akey.Key,
			Status: http.StatusForbidden,
		},
		{
			Name:   "no token",
			Path:   "/api/test/user",
			Token:  "",
			Status: http.StatusUnauthorized,
		},
	} {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()

			assert := require.New(t)

			req := httptest.NewRequest(http.MethodGet, tc.Path, nil)
			req.Header.Set("Authorization", "Bearer "+tc.Token)
			rec := httptest.NewRecorder()
			server.ServeHTTP(rec, req)
			t.Log(rec.Body.String())
			assert.Equal(tc.Status, rec.Code)
		})
	}
}
