package user

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/governortest"
	"xorkevin.dev/governor/service/authzacl"
	"xorkevin.dev/governor/service/dbsql/dbsqltest"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/gate"
	"xorkevin.dev/governor/service/gate/apikey"
	"xorkevin.dev/governor/service/gate/gatetest"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/user/approvalmodel"
	"xorkevin.dev/governor/service/user/resetmodel"
	"xorkevin.dev/governor/service/user/sessionmodel"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/governor/util/kjson"
	"xorkevin.dev/klog"
)

func TestUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("relies on db")
	}

	t.Parallel()

	assert := require.New(t)

	gateClient, err := gatetest.NewClient()
	assert.NoError(err)
	systoken, err := gateClient.GenToken(gate.KeySubSystem, time.Hour, "")
	assert.NoError(err)
	gateClient.Token = systoken

	server := governortest.NewTestServer(t, map[string]any{
		"gate": map[string]any{
			"tokensecret": "tokensecret",
		},
		"user": map[string]any{
			"otpkey": "otpkey",
		},
	}, map[string]any{
		"data": map[string]any{
			"tokensecret": map[string]any{
				"keys":    []string{gateClient.KeyStr},
				"extkeys": []string{gateClient.ExtKeyStr},
			},
			"otpkey": map[string]any{
				"secrets": []string{},
			},
		},
	}, nil)

	db := dbsqltest.NewStatic(t)
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
		ratelimiter,
		g,
	)

	server.Register("gate", "/null/gate", g)
	server.Register("user", "/u", users)

	assert.NoError(server.Setup(context.Background(), governor.Flags{}, klog.Discard{}))
	assert.NoError(server.Start(context.Background(), governor.Flags{}, klog.Discard{}))

	term := governortest.NewTestTerm()
	var out bytes.Buffer
	term.Stdout = &out
	client := governortest.NewTestClient(t, server, nil, term)

	userClient := NewCmdClient(gateClient)
	client.Register("user", "/u", &governor.CmdDesc{
		Usage: "user",
		Short: "user",
		Long:  "user",
	}, userClient)

	assert.NoError(client.Init(governor.ClientFlags{}, klog.Discard{}))

	userClient.addAdminReq = reqAddAdmin{
		Username:  "xorkevin",
		Password:  "password",
		Email:     "test@example.com",
		Firstname: "Kevin",
		Lastname:  "Wang",
	}
	assert.NoError(userClient.addAdmin(nil))

	adminUserid := strings.TrimSpace(out.String())
	out.Reset()

	userClient.getUserFlags.userid = adminUserid
	assert.NoError(userClient.getUser(nil))

	var body ResUserGetPublic
	assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
	out.Reset()

	assert.Equal(ResUserGetPublic{
		Userid:       adminUserid,
		Username:     "xorkevin",
		FirstName:    "Kevin",
		LastName:     "Wang",
		CreationTime: body.CreationTime,
	}, body)
}
