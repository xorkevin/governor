package gate

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

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

	assert.NoError(gateClient.validateToken(nil))
	var claims Claims
	assert.NoError(json.Unmarshal(out.Bytes(), &claims))
	assert.Equal(KeySubSystem, claims.Subject)
	assert.Equal("", claims.Kind)
	assert.Equal("", claims.Scope)

	rsakey, err := rsasig.NewConfig()
	assert.NoError(err)
	rsastr, err := rsakey.String()
	assert.NoError(err)

	server := governortest.NewTestServer(t, map[string]any{
		"gate": map[string]any{
			"tokensecret": "tokensecret",
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
	_, err = keyset.InsertKey(context.Background(), "test-user-1", "test-scope", "", "")
	assert.NoError(err)

	g := New(&acl, &keyset)
	server.Register("gate", "/null/gate", g)

	assert.NoError(server.Start(context.Background(), governor.Flags{}, klog.Discard{}))

	jwks, err := g.GetJWKS(context.Background())
	assert.NoError(err)
	assert.Len(jwks.Keys, 1)
	assert.Equal(string(jose.RS256), jwks.Keys[0].Algorithm)
}
