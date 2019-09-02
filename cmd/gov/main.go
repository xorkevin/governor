package main

import (
	"fmt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/kvstore"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/state/model"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/service/user/gate"
)

var (
	// GitHash is the git hash to be passed in at compile time
	GitHash string
)

func main() {
	dbService := db.New()
	stateService := statemodel.New(dbService)

	gov := governor.New(governor.ConfigOpts{
		DefaultFile: "config",
		Appname:     "governor",
		Version:     "v0.2.0",
		VersionHash: GitHash,
		EnvPrefix:   "gov",
	}, stateService)

	kvService := kvstore.New()
	objstoreService := objstore.New()
	msgqueueService, err := msgqueue.New()
	if err != nil {
		fmt.Printf("Failed to create msgqueue: %s\n", err.Error())
		return
	}
	templateService := template.New()
	mailService := mail.New(templateService, msgqueueService)
	gateService := gate.New()

	gov.Register("database", "/null", dbService)
	gov.Register("kvstore", "/null", kvService)
	gov.Register("objstore", "/null", objstoreService)
	gov.Register("msgqueue", "/null", msgqueueService)
	gov.Register("template", "/null", templateService)
	gov.Register("mail", "/null", mailService)
	gov.Register("gate", "/null", gateService)

	gov.Start()
}
