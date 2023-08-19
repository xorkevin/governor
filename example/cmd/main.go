package main

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit"
	"xorkevin.dev/governor/service/conduit/dmmodel"
	"xorkevin.dev/governor/service/conduit/friendinvmodel"
	"xorkevin.dev/governor/service/conduit/friendmodel"
	"xorkevin.dev/governor/service/conduit/gdmmodel"
	"xorkevin.dev/governor/service/conduit/msgmodel"
	"xorkevin.dev/governor/service/conduit/servermodel"
	"xorkevin.dev/governor/service/courier"
	"xorkevin.dev/governor/service/courier/couriermodel"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/eventsapi"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mailinglist"
	"xorkevin.dev/governor/service/mailinglist/mailinglistmodel"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile"
	"xorkevin.dev/governor/service/profile/profilemodel"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/apikey"
	"xorkevin.dev/governor/service/user/apikey/apikeymodel"
	"xorkevin.dev/governor/service/user/approvalmodel"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/oauth"
	"xorkevin.dev/governor/service/user/oauth/oauthappmodel"
	"xorkevin.dev/governor/service/user/oauth/oauthconnmodel"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/service/user/org/orgmodel"
	"xorkevin.dev/governor/service/user/resetmodel"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/service/user/role/rolemodel"
	"xorkevin.dev/governor/service/user/roleinvmodel"
	"xorkevin.dev/governor/service/user/sessionmodel"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/service/user/usermodel"
	"xorkevin.dev/governor/service/ws"
)

func main() {
	vcsinfo := governor.ReadVCSBuildInfo()
	opts := governor.Opts{
		Appname: "governor",
		Version: governor.Version{
			Num:  vcsinfo.ModVersion,
			Hash: vcsinfo.VCSStr(),
		},
		Description:   "Governor is a web server with user and auth capabilities",
		DefaultFile:   "governor",
		EnvPrefix:     "gov",
		ClientDefault: "client",
		ClientPrefix:  "govc",
	}

	gov := governor.New(opts)

	gov.Register("database", "/null/db", db.New())
	gov.Register("kvstore", "/null/kv", kvstore.New())
	gov.Register("objstore", "/null/obj", objstore.New())
	gov.Register("pubsub", "/null/pubsub", pubsub.New())
	gov.Register("events", "/null/events", events.NewNats())
	gov.Register("template", "/null/tpl", template.New())
	{
		inj := gov.Injector()
		objstore.NewBucketInCtx(inj, "mail")
		gov.Register("mail", "/null/mail", mail.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		kvstore.NewSubtreeInCtx(inj, "ratelimit")
		gov.Register("ratelimit", "/null/ratelimit", ratelimit.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		rolemodel.NewInCtx(inj, "userroles")
		kvstore.NewSubtreeInCtx(inj, "roles")
		gov.Register("role", "/null/role", role.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		apikeymodel.NewInCtx(inj, "userapikeys")
		kvstore.NewSubtreeInCtx(inj, "apikeys")
		gov.Register("apikey", "/null/apikey", apikey.NewCtx(inj))
	}
	gov.Register("token", "/null/token", token.New())
	gov.Register("gate", "/null/gate", gate.NewCtx(gov.Injector()))
	gov.Register("eventsapi", "/eventsapi", eventsapi.NewCtx(gov.Injector()))
	{
		inj := gov.Injector()
		ratelimit.NewSubtreeInCtx(inj, "ws")
		gov.Register("ws", "/ws", ws.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		usermodel.NewInCtx(inj, "users")
		sessionmodel.NewInCtx(inj, "usersessions")
		approvalmodel.NewInCtx(inj, "userapprovals")
		roleinvmodel.NewInCtx(inj, "userroleinvitations")
		resetmodel.NewInCtx(inj, "userresets")
		kvstore.NewSubtreeInCtx(inj, "user")
		ratelimit.NewSubtreeInCtx(inj, "user")
		gov.Register("user", "/u", user.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		orgmodel.NewInCtx(inj, "userorgs", "userorgmembers", "userorgmods")
		ratelimit.NewSubtreeInCtx(inj, "org")
		gov.Register("org", "/org", org.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		oauthappmodel.NewInCtx(inj, "oauthapps")
		oauthconnmodel.NewInCtx(inj, "oauthconnections")
		kvstore.NewSubtreeInCtx(inj, "oauth")
		objstore.NewBucketInCtx(inj, "oauth-app-logo")
		ratelimit.NewSubtreeInCtx(inj, "oauth")
		gov.Register("oauth", "/oauth", oauth.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		profilemodel.NewInCtx(inj, "profiles")
		objstore.NewBucketInCtx(inj, "profile-image")
		ratelimit.NewSubtreeInCtx(inj, "profile")
		gov.Register("profile", "/profile", profile.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		couriermodel.NewInCtx(inj, "courierlinks", "courierbrands")
		kvstore.NewSubtreeInCtx(inj, "courier")
		objstore.NewBucketInCtx(inj, "link-qr-image")
		ratelimit.NewSubtreeInCtx(inj, "courier")
		gov.Register("courier", "/courier", courier.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		friendmodel.NewInCtx(inj, "friends")
		friendinvmodel.NewInCtx(inj, "friendinvitations")
		dmmodel.NewInCtx(inj, "dms")
		gdmmodel.NewInCtx(inj, "gdms", "gdmmembers", "gdmassocs")
		servermodel.NewInCtx(inj, "servers", "serverchannels", "serverpresence")
		msgmodel.NewInCtx(inj, "chatmsgs")
		kvstore.NewSubtreeInCtx(inj, "conduit")
		ratelimit.NewSubtreeInCtx(inj, "conduit")
		gov.Register("conduit", "/conduit", conduit.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		mailinglistmodel.NewInCtx(inj, "mailinglists", "mailinglistmembers", "mailinglistmsgs", "mailinglistsentmsgs", "mailinglisttree")
		objstore.NewBucketInCtx(inj, "mailinglist")
		ratelimit.NewSubtreeInCtx(inj, "mailinglist")
		gov.Register("mailinglist", "/mailinglist", mailinglist.NewCtx(inj))
	}

	client := governor.NewClient(opts)
	client.Register("token", "/null/token", &governor.CmdDesc{
		Usage: "token",
		Short: "manage tokens",
		Long:  "manage tokens",
	}, token.NewCmdClient())
	client.Register("gate", "/null/gate", nil, gate.NewCmdClient())
	client.Register("events", "/eventsapi", &governor.CmdDesc{
		Usage: "events",
		Short: "interact with events",
		Long:  "interact with events",
	}, eventsapi.NewCmdClientCtx(client.Injector()))
	client.Register("user", "/u", &governor.CmdDesc{
		Usage: "user",
		Short: "manage users",
		Long:  "manage users",
	}, user.NewCmdClientCtx(client.Injector()))

	cmd := governor.NewCmd(opts, governor.CmdOpts{}, gov, client)
	cmd.Execute()
}
