package gate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/governortest"
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
	gateClient.tokenFlags.scope = ScopeAll
	assert.NoError(gateClient.genToken(nil))
	assert.NotNil(fsys.Fsys["token.txt"])

	assert.NoError(gateClient.validateToken(nil))
	var claims Claims
	assert.NoError(json.Unmarshal(out.Bytes(), &claims))
	assert.Equal(KeySubSystem, claims.Subject)
	assert.Equal(KindAccess, claims.Kind)
	assert.Equal(ScopeAll, claims.Scope)

	governortest.NewTestServer(t, map[string]any{
		"gate": map[string]any{
			"tokensecret": "tokensecret",
		},
	}, map[string]any{
		"data": map[string]any{
			"tokensecret": string(bytes.TrimSpace(fsys.Fsys["key.txt"].Data)),
		},
	}, nil)
}
