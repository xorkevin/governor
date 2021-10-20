package main

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/conduit"
	conduitchatmodel "xorkevin.dev/governor/service/conduit/chat/model"
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
	statemodel "xorkevin.dev/governor/service/state/model"
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
)

var (
	// GitHash is the git hash to be passed in at compile time
	GitHash string
)

func main() {
	opts := governor.Opts{
		Version: governor.Version{
			Num:  "v0.3",
			Hash: GitHash,
		},
		Appname:       "governor",
		Description:   "Governor is a web server with user and auth capabilities",
		DefaultFile:   "config",
		ClientDefault: "client",
		ClientPrefix:  "govc",
		EnvPrefix:     "gov",
	}

	dbService := db.New()
	stateService := statemodel.New(dbService)

	gov := governor.New(opts, stateService)

	gov.Register("database", "/null/db", dbService)
	gov.Register("kvstore", "/null/kv", kvstore.New())
	gov.Register("objstore", "/null/obj", objstore.New())
	gov.Register("events", "/null/events", events.New())
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
		rolemodel.NewInCtx(inj)
		kvstore.NewSubtreeInCtx(inj, "roles")
		gov.Register("role", "/null/role", role.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		apikeymodel.NewInCtx(inj)
		kvstore.NewSubtreeInCtx(inj, "apikeys")
		gov.Register("apikey", "/null/apikey", apikey.NewCtx(inj))
	}
	gov.Register("token", "/null/token", token.New())
	gov.Register("gate", "/null/gate", gate.NewCtx(gov.Injector()))
	{
		inj := gov.Injector()
		usermodel.NewInCtx(inj)
		sessionmodel.NewInCtx(inj)
		approvalmodel.NewInCtx(inj)
		invitationmodel.NewInCtx(inj)
		resetmodel.NewInCtx(inj)
		kvstore.NewSubtreeInCtx(inj, "user")
		ratelimit.NewSubtreeInCtx(inj, "user")
		gov.Register("user", "/u", user.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		orgmodel.NewInCtx(inj)
		gov.Register("org", "/org", org.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		oauthmodel.NewInCtx(inj)
		connmodel.NewInCtx(inj)
		kvstore.NewSubtreeInCtx(inj, "oauth")
		objstore.NewBucketInCtx(inj, "oauth-app-logo")
		gov.Register("oauth", "/oauth", oauth.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		profilemodel.NewInCtx(inj)
		objstore.NewBucketInCtx(inj, "profile-image")
		gov.Register("profile", "/profile", profile.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		couriermodel.NewInCtx(inj)
		kvstore.NewSubtreeInCtx(inj, "courier")
		objstore.NewBucketInCtx(inj, "link-qr-image")
		gov.Register("courier", "/courier", courier.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		conduitchatmodel.NewInCtx(inj)
		gov.Register("conduit", "/conduit", conduit.NewCtx(inj))
	}
	{
		inj := gov.Injector()
		mailinglistmodel.NewInCtx(inj)
		objstore.NewBucketInCtx(inj, "mailinglist")
		gov.Register("mailinglist", "/mailinglist", mailinglist.NewCtx(inj))
	}

	cmd := governor.NewCmd(opts, gov, governor.NewClient(opts))
	cmd.Execute()
}
