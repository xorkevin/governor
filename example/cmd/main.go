package main

import (
	"context"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/courier"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/profile"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/pubsub"
	"xorkevin.dev/governor/service/state/model"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/apikey"
	"xorkevin.dev/governor/service/user/apikey/model"
	"xorkevin.dev/governor/service/user/approval/model"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/service/user/oauth"
	"xorkevin.dev/governor/service/user/oauth/connection/model"
	"xorkevin.dev/governor/service/user/oauth/model"
	"xorkevin.dev/governor/service/user/org"
	"xorkevin.dev/governor/service/user/org/model"
	"xorkevin.dev/governor/service/user/role"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/service/user/session/model"
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

	ctx := context.Background()

	dbService := db.New()
	stateService := statemodel.New(dbService)

	gov := governor.New(opts, stateService)

	gov.Register("database", "/null/db", dbService)
	kvService := kvstore.New()
	gov.Register("kvstore", "/null/kv", kvService)
	objstoreService := objstore.New()
	gov.Register("objstore", "/null/obj", objstoreService)

	{
		msgqueueService := msgqueue.New()
		gov.Register("msgqueue", "/null/msg", msgqueueService)
		ctx = msgqueue.SetCtxMsgqueue(ctx, msgqueueService)
	}
	{
		pubsubService := pubsub.New()
		gov.Register("pubsub", "/null/pubsub", pubsubService)
		ctx = pubsub.SetCtxPubsub(ctx, pubsubService)
	}
	{
		templateService := template.New()
		gov.Register("template", "/null/tpl", templateService)
		ctx = template.SetCtxTemplate(ctx, templateService)
	}
	{
		mailService, err := mail.NewCtx(ctx)
		governor.Must(err)
		gov.Register("mail", "/null/mail", mailService)
		ctx = mail.SetCtxMail(ctx, mailService)
	}
	{
		roleModel := rolemodel.New(dbService)
		roleService := role.New(roleModel, kvService.Subtree("roles"))
		gov.Register("role", "/null/role", roleService)
		ctx = role.SetCtxRole(ctx, roleService)
	}
	{
		apikeyModel := apikeymodel.New(dbService)
		apikeyService := apikey.New(apikeyModel, kvService.Subtree("apikeys"))
		gov.Register("apikey", "/null/apikey", apikeyService)
		ctx = apikey.SetCtxApikey(ctx, apikeyService)
	}
	{
		tokenService := token.New()
		gov.Register("token", "/null/token", tokenService)
		ctx = token.SetCtxTokenizer(ctx, tokenService)
	}
	{
		gateService, err := gate.NewCtx(ctx)
		governor.Must(err)
		gov.Register("gate", "/null/gate", gateService)
		ctx = gate.SetCtxGate(ctx, gateService)
	}
	{
		c := usermodel.SetCtxRepo(ctx, usermodel.New(dbService))
		c = sessionmodel.SetCtxRepo(c, sessionmodel.New(dbService))
		c = approvalmodel.SetCtxRepo(c, approvalmodel.New(dbService))
		c = approvalmodel.SetCtxRepo(c, approvalmodel.New(dbService))
		c = kvstore.SetCtxKVStore(c, kvService.Subtree("user"))
		userService, err := user.NewCtx(c)
		governor.Must(err)
		gov.Register("user", "/u", userService)
		ctx = user.SetCtxUser(ctx, userService)
	}
	{
		c := orgmodel.SetCtxRepo(ctx, orgmodel.New(dbService))
		orgService, err := org.NewCtx(c)
		governor.Must(err)
		gov.Register("org", "/org", orgService)
		ctx = org.SetCtxOrg(ctx, orgService)
	}
	{
		c := oauthmodel.SetCtxRepo(ctx, oauthmodel.New(dbService))
		c = connectionmodel.SetCtxRepo(c, connectionmodel.NewRepo(dbService))
		c = objstore.SetCtxBucket(c, objstoreService.GetBucket("oauth-app-logo"))
		c = kvstore.SetCtxKVStore(c, kvService.Subtree("oauth"))
		oauthService, err := oauth.NewCtx(c)
		governor.Must(err)
		gov.Register("oauth", "/oauth", oauthService)
		ctx = oauth.SetCtxOAuth(ctx, oauthService)
	}
	{
		c := profilemodel.SetCtxRepo(ctx, profilemodel.New(dbService))
		c = objstore.SetCtxBucket(c, objstoreService.GetBucket("profile-image"))
		profileService, err := profile.NewCtx(c)
		governor.Must(err)
		gov.Register("profile", "/profile", profileService)
		ctx = profile.SetCtxProfile(ctx, profileService)
	}
	{
		c := couriermodel.SetCtxRepo(ctx, couriermodel.New(dbService))
		c = objstore.SetCtxBucket(c, objstoreService.GetBucket("link-qr-image"))
		c = kvstore.SetCtxKVStore(c, kvService.Subtree("courier"))
		courierService, err := courier.NewCtx(c)
		governor.Must(err)
		gov.Register("courier", "/courier", courierService)
		ctx = courier.SetCtxCourier(ctx, courierService)
	}

	cmd := governor.NewCmd(opts, gov, governor.NewClient(opts))
	cmd.Execute()
}
