package maimln

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

	gov := governor.New(opts, nil)

	d := db.New()
	gov.Register("database", "/null/db", d)
	kv := kvstore.New()
	gov.Register("kvstore", "/null/kv", kv)
	obj := objstore.New()
	gov.Register("objstore", "/null/obj", obj)
	ps := pubsub.New()
	gov.Register("pubsub", "/null/pubsub", ps)
	ev := events.NewNats()
	gov.Register("events", "/null/events", ev)
	tpl := template.New()
	gov.Register("template", "/null/tpl", tpl)
	ml := mail.New(tpl, ev, obj.GetBucket("mail"))
	gov.Register("mail", "/null/mail", ml)
	ratelim := ratelimit.New(kv.Subtree("ratelimit"))
	gov.Register("ratelimit", "/null/ratelimit", ratelim)
	rolesvc := role.New(rolemodel.New(d, "userroles"), kv.Subtree("roles"), ev)
	gov.Register("role", "/null/role", rolesvc)
	apikeysvc := apikey.New(apikeymodel.New(d, "userapikeys"))
	gov.Register("apikey", "/null/apikey", apikeysvc)
	tokensvc := token.New()
	gov.Register("token", "/null/token", tokensvc)
	g := gate.New(rolesvc, apikeysvc, tokensvc)
	gov.Register("gate", "/null/gate", g)
	gov.Register("eventsapi", "/eventsapi", eventsapi.New(ps, g))
	wssvc := ws.New(ps, ratelim.Subtree("ws"), g)
	gov.Register("ws", "/ws", wssvc)
	usersvc := user.New(
		usermodel.New(d, "users"),
		sessionmodel.New(d, "usersessions"),
		approvalmodel.New(d, "userapprovals"),
		roleinvmodel.New(d, "userroleinvitations"),
		resetmodel.New(d, "userresets"),
		rolesvc,
		apikeysvc,
		kv.Subtree("user"),
		ps,
		ev,
		ml,
		ratelim.Subtree("user"),
		tokensvc,
		g,
	)
	gov.Register("user", "/u", usersvc)
	orgsvc := org.New(
		orgmodel.New(d, "userorgs", "userorgmembers", "userorgmods"),
		usersvc,
		ev,
		ratelim.Subtree("org"),
		g,
	)
	gov.Register("org", "/org", orgsvc)
	gov.Register("oauth", "/oauth", oauth.New(
		oauthappmodel.New(d, "oauthapps"),
		oauthconnmodel.New(d, "oauthconnections"),
		tokensvc,
		kv.Subtree("oauth"),
		obj.GetBucket("oauth-app-logo"),
		usersvc,
		ev,
		ratelim.Subtree("oauth"),
		g,
	))
	gov.Register("profile", "/profile", profile.New(
		profilemodel.New(d, "profiles"),
		obj.GetBucket("profile-image"),
		usersvc,
		ratelim.Subtree("profile"),
		g,
	))
	gov.Register("courier", "/courier", courier.New(
		couriermodel.New(d, "courierlinks", "courierbrands"),
		kv.Subtree("courier"),
		usersvc,
		orgsvc,
		ratelim.Subtree("courier"),
		g,
	))
	gov.Register("conduit", "/conduit", conduit.New(
		friendmodel.New(d, "friends"),
		friendinvmodel.New(d, "friendinvitations"),
		dmmodel.New(d, "dms"),
		gdmmodel.New(d, "gdms", "gdmmembers", "gdmassocs"),
		servermodel.New(d, "servers", "serverchannels", "serverpresence"),
		msgmodel.New(d, "chatmsgs"),
		kv.Subtree("conduit"),
		usersvc,
		ps,
		ev,
		wssvc,
		ratelim.Subtree("conduit"),
		g,
	))
	gov.Register("mailinglist", "/mailinglist", mailinglist.New(
		mailinglistmodel.New(d, "mailinglists", "mailinglistmembers", "mailinglistmsgs", "mailinglistsentmsgs", "mailinglisttree"),
		obj.GetBucket("mailinglist"),
		ev,
		usersvc,
		orgsvc,
		ml,
		ratelim.Subtree("mailinglist"),
		g,
	))

	client := governor.NewClient(opts, nil)
	client.Register("token", "/null/token", &governor.CmdDesc{
		Usage: "token",
		Short: "manage tokens",
		Long:  "manage tokens",
	}, token.NewCmdClient())
	gateclient := gate.NewCmdClient()
	client.Register("gate", "/null/gate", nil, gateclient)
	client.Register("events", "/eventsapi", &governor.CmdDesc{
		Usage: "events",
		Short: "interact with events",
		Long:  "interact with events",
	}, eventsapi.NewCmdClient(gateclient))
	client.Register("user", "/u", &governor.CmdDesc{
		Usage: "user",
		Short: "manage users",
		Long:  "manage users",
	}, user.NewCmdClient(gateclient))

	cmd := governor.NewCmd(opts, nil, gov, client)
	cmd.Execute()
}
