package main

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit"
	dmmodel "xorkevin.dev/governor/service/conduit/dm/model"
	friendinvmodel "xorkevin.dev/governor/service/conduit/friend/invitation/model"
	friendmodel "xorkevin.dev/governor/service/conduit/friend/model"
	gdmmodel "xorkevin.dev/governor/service/conduit/gdm/model"
	msgmodel "xorkevin.dev/governor/service/conduit/msg/model"
	"xorkevin.dev/governor/service/courier"
	couriermodel "xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/events"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mailinglist"
	mailinglistmodel "xorkevin.dev/governor/service/mailinglist/model"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile"
	profilemodel "xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/ratelimit"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/apikey"
	apikeymodel "xorkevin.dev/governor/service/user/apikey/model"
	approvalmodel "xorkevin.dev/governor/service/user/approval/model"
	"xorkevin.dev/governor/service/user/gate"
	usermodel "xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/service/user/oauth"
	connmodel "xorkevin.dev/governor/service/user/oauth/connection/model"
	oauthmodel "xorkevin.dev/governor/service/user/oauth/model"
	"xorkevin.dev/governor/service/user/org"
	orgmodel "xorkevin.dev/governor/service/user/org/model"
	resetmodel "xorkevin.dev/governor/service/user/reset/model"
	"xorkevin.dev/governor/service/user/role"
	invitationmodel "xorkevin.dev/governor/service/user/role/invitation/model"
	rolemodel "xorkevin.dev/governor/service/user/role/model"
	sessionmodel "xorkevin.dev/governor/service/user/session/model"
	"xorkevin.dev/governor/service/user/token"
	"xorkevin.dev/governor/service/ws"
)

func main() {
	vcsinfo := governor.ReadVCSBuildInfo()

	gov := governor.New(governor.Opts{
		Version: governor.Version{
			Num:  vcsinfo.ModVersion,
			Hash: vcsinfo.VCSStr(),
		},
		Appname:       "governor",
		Description:   "Governor is a web server with user and auth capabilities",
		DefaultFile:   "config",
		ClientDefault: "client",
		ClientPrefix:  "govc",
		EnvPrefix:     "gov",
	})

	gov.Register("database", "/null/db", db.New())
	gov.Register("kvstore", "/null/kv", kvstore.New())
	gov.Register("objstore", "/null/obj", objstore.New())
	gov.Register("events", "/events", events.New())
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
	gov.Register("ws", "/ws", ws.NewCtx(gov.Injector()))
	{
		inj := gov.Injector()
		usermodel.NewInCtx(inj, "users")
		sessionmodel.NewInCtx(inj, "usersessions")
		approvalmodel.NewInCtx(inj, "userapprovals")
		invitationmodel.NewInCtx(inj, "userroleinvitations")
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
		oauthmodel.NewInCtx(inj, "oauthapps")
		connmodel.NewInCtx(inj, "oauthconnections")
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
		msgmodel.NewInCtx(inj, "chatmsgs")
		kvstore.NewSubtreeInCtx(inj, "conduit")
		gov.Register("conduit", "/conduit", conduit.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		mailinglistmodel.NewInCtx(inj, "mailinglists", "mailinglistmembers", "mailinglistmsgs", "mailinglistsentmsgs", "mailinglisttree")
		objstore.NewBucketInCtx(inj, "mailinglist")
		gov.Register("mailinglist", "/mailinglist", mailinglist.NewCtx(inj))
	}

	cmd := governor.NewCmd(opts, gov, governor.NewClient(opts))
	cmd.Execute()
}
