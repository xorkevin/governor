package main

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/msgqueue/store"
	"xorkevin.dev/governor/service/state/model"
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
		Appname:     "govsetup",
		Description: "govsetup initializes preliminary resources for governor dependencies",
		Version:     "v0.2",
		VersionHash: GitHash,
		EnvPrefix:   "govsetup",
	}, stateService)

	msgqueueStore := msgqueuestore.New(dbService)

	gov.Register("msgstoredb", "/null", dbService)
	gov.Register("msgstore", "/null", msgqueueStore)

	gov.Execute()
}
