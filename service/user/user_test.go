package user

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/governortest"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/dbsql/dbsqltest"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/service/gate/apikey"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/approvalmodel"
	"xorkevin.dev/governor/service/user/resetmodel"
	"xorkevin.dev/governor/service/user/sessionmodel"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/klog"
)

func TestUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("relies on db")
	}

	t.Parallel()

	assert := require.New(t)

	server := governortest.NewTestServer(t, map[string]any{
		"gate": map[string]any{
			"tokensecret": "tokensecret",
			"issuer":      "test-issuer",
		},
	}, map[string]any{
		"data": map[string]any{},
	}, nil)

	db := dbsqltest.NewTestStatic(t)
	acl := authzacl.ACLSet{
		Set: map[authzacl.Relation]struct{}{},
	}
	keyset := apikey.KeySet{
		Set: map[string]apikey.MemKey{},
	}
	kvmap := kvstore.NewMap()
	psmux := pubsub.NewMuxChan()
	evmux := events.NewMuxChan()
	maillog := mail.MemLog{}
	ratelimiter := ratelimit.Unrestricted{}
	g := gate.New(&acl, &keyset)
	users := New(
		usermodel.New(db, "users"),
		sessionmodel.New(db, "sessions"),
		approvalmodel.New(db, "userapprovals"),
		resetmodel.New(db, "userresets"),
		&acl,
		&keyset,
		kvmap,
		psmux,
		evmux,
		&maillog,
		&ratelimiter,
		g,
	)

	server.Register("gate", "/null/gate", g)
	server.Register("user", "/u", users)

	assert.NoError(server.Setup(context.Background(), governor.Flags{}, klog.Discard{}))
}
