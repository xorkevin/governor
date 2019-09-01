package main

import (
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/db"
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
		Appname:     "governor",
		Version:     "v0.2.0",
		VersionHash: GitHash,
		EnvPrefix:   "gov",
	}, stateService)

	gov.Register("database", "/null", dbService)

	gov.Start()
}
