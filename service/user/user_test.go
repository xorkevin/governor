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
	"xorkevin.dev/governor/service/template"
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
			"edit": map[string]any{
				"newUserApproval": true,
			},
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
	ratelimiter := ratelimit.Unlimited{}
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

	userClient.reqUserPost = reqUserPost{
		Username:  "xorkevin",
		Password:  "password",
		Email:     "test@example.com",
		FirstName: "Kevin",
		LastName:  "Wang",
	}
	assert.NoError(userClient.addAdmin(nil))

	adminUserid := strings.TrimSpace(out.String())
	out.Reset()

	adminToken, err := gateClient.GenToken(adminUserid, time.Hour, "")
	assert.NoError(err)
	gateClient.Token = adminToken

	{
		userClient.getUserFlags = getUserFlags{
			userid:  adminUserid,
			private: true,
		}
		assert.NoError(userClient.getUser(nil))

		var body ResUserGet
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Equal(ResUserGet{
			ResUserGetPublic: ResUserGetPublic{
				Userid:       adminUserid,
				Username:     "xorkevin",
				FirstName:    "Kevin",
				LastName:     "Wang",
				CreationTime: body.CreationTime,
			},
			Email:      "test@example.com",
			OTPEnabled: false,
		}, body)
	}

	userClient.reqUserPost = reqUserPost{
		Username:  "xorkevin2",
		Password:  "password",
		Email:     "test2@example.com",
		FirstName: "Test",
		LastName:  "User",
	}
	assert.NoError(userClient.createUser(nil))

	regularUserid := strings.TrimSpace(out.String())
	out.Reset()

	{
		userClient.listFlags = listFlags{
			amount: 8,
			offset: 0,
		}
		assert.NoError(userClient.getApprovals(nil))

		var body resApprovals
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Len(body.Approvals, 1)
		assert.Equal(regularUserid, body.Approvals[0].Userid)
		assert.False(body.Approvals[0].Approved)
	}

	{
		userClient.useridFlags = useridFlags{
			userid: regularUserid,
		}
		assert.NoError(userClient.acceptApproval(nil))
	}

	{
		userClient.listFlags = listFlags{
			amount: 8,
			offset: 0,
		}
		assert.NoError(userClient.getApprovals(nil))

		var body resApprovals
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Len(body.Approvals, 1)
		assert.Equal(regularUserid, body.Approvals[0].Userid)
		assert.True(body.Approvals[0].Approved)
	}

	{
		assert.Len(maillog.Records, 1)
		assert.Equal("test2@example.com", maillog.Records[0].To[0].Address)
		assert.Equal(template.KindLocal, maillog.Records[0].Tpl.Kind)
		assert.Equal("newuser", maillog.Records[0].Tpl.Name)
		key := maillog.Records[0].TplData["Key"]

		userClient.useridFlags = useridFlags{
			userid: regularUserid,
		}
		userClient.keyFlags = keyFlags{
			key: key,
		}
		assert.NoError(userClient.commitUser(nil))

		assert.Equal(regularUserid, strings.TrimSpace(out.String()))
		out.Reset()
	}

	{
		userClient.listFlags = listFlags{
			amount: 8,
			offset: 0,
		}
		assert.NoError(userClient.getApprovals(nil))

		var body resApprovals
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Len(body.Approvals, 0)
	}

	{
		userClient.useridFlags = useridFlags{
			userid: "",
		}
		userClient.roleFlags = roleFlags{
			mod: false,
		}
		userClient.listFlags = listFlags{
			amount: 8,
		}
		assert.NoError(userClient.getRoles(nil))

		var body resUserRoles
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Equal([]string{gate.RoleAdmin, gate.RoleUser}, body.Roles)
	}

	{
		userClient.useridFlags = useridFlags{
			userid: regularUserid,
		}
		userClient.roleFlags = roleFlags{
			mod: false,
		}
		userClient.listFlags = listFlags{
			amount: 8,
		}
		assert.NoError(userClient.getRoles(nil))

		var body resUserRoles
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Equal([]string{gate.RoleUser}, body.Roles)
	}

	{
		userClient.useridFlags = useridFlags{
			userid: "",
		}
		userClient.roleFlags = roleFlags{
			mod:       false,
			intersect: gate.RoleAdmin,
		}
		assert.NoError(userClient.intersectRoles(nil))

		var body resUserRoles
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Equal([]string{gate.RoleAdmin}, body.Roles)
	}

	{
		userClient.roleFlags = roleFlags{
			mod:  false,
			name: gate.RoleAdmin,
		}
		userClient.listFlags = listFlags{
			amount: 8,
		}
		assert.NoError(userClient.getRoleMembers(nil))

		var body resUserList
		assert.NoError(kjson.Unmarshal(out.Bytes(), &body))
		out.Reset()

		assert.Equal([]string{adminUserid}, body.Userids)
	}
}
