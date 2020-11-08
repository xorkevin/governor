package main

import (
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

	dbService := db.New()
	stateService := statemodel.New(dbService)

	gov := governor.New(opts, stateService)

	kvService := kvstore.New()
	objstoreService := objstore.New()
	msgqueueService := msgqueue.New()
	pubsubService := pubsub.New()
	templateService := template.New()
	mailService := mail.New(templateService, msgqueueService)
	roleModel := rolemodel.New(dbService)
	roleService := role.New(roleModel, kvService.Subtree("roles"))
	apikeyModel := apikeymodel.New(dbService)
	apikeyService := apikey.New(apikeyModel, kvService.Subtree("apikeys"))
	tokenService := token.New()
	gateService := gate.New(roleService, apikeyService, tokenService)
	userModel := usermodel.New(dbService)
	sessionModel := sessionmodel.New(dbService)
	approvalModel := approvalmodel.New(dbService)
	userService := user.New(userModel, sessionModel, approvalModel, roleService, apikeyService, kvService.Subtree("user"), msgqueueService, mailService, tokenService, gateService)
	orgModel := orgmodel.New(dbService)
	orgService := org.New(orgModel, roleService, gateService)
	oauthModel := oauthmodel.New(dbService)
	oauthConnectionModel := connectionmodel.NewRepo(dbService)
	oauthService := oauth.New(oauthModel, oauthConnectionModel, tokenService, objstoreService.GetBucket("oauth-app-logo"), kvService.Subtree("oauth"), gateService)
	profileModel := profilemodel.New(dbService)
	profileService := profile.New(profileModel, objstoreService.GetBucket("profile-image"), msgqueueService, gateService)
	courierModel := couriermodel.New(dbService)
	courierService := courier.New(courierModel, objstoreService.GetBucket("link-qr-image"), kvService.Subtree("courier"), gateService)

	gov.Register("database", "/null/db", dbService)
	gov.Register("kvstore", "/null/kv", kvService)
	gov.Register("objstore", "/null/obj", objstoreService)
	gov.Register("msgqueue", "/null/msg", msgqueueService)
	gov.Register("pubsub", "/null/pubsub", pubsubService)
	gov.Register("template", "/null/tpl", templateService)
	gov.Register("mail", "/null/mail", mailService)
	gov.Register("role", "/null/role", roleService)
	gov.Register("apikey", "/null/apikey", apikeyService)
	gov.Register("token", "/null/token", tokenService)
	gov.Register("gate", "/null/gate", gateService)
	gov.Register("user", "/u", userService)
	gov.Register("org", "/org", orgService)
	gov.Register("oauth", "/oauth", oauthService)
	gov.Register("profile", "/profile", profileService)
	gov.Register("courier", "/courier", courierService)

	cmd := governor.NewCmd(opts, gov, governor.NewClient(opts))
	cmd.Execute()
}
