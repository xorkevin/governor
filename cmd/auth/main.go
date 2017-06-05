package main

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf"
	"github.com/hackform/governor/service/user"
)

func main() {
	config, err := governor.NewConfig("auth")
	if err != nil {
		fmt.Printf("error reading config: %s\n", err)
		return
	}

	g, err := governor.New(config)
	if err != nil {
		return
	}

	log := g.Logger()

	db, err := governor.NewDB(&config)
	if err != nil {
		log.Errorf("error creating DB: %s\n", err)
		return
	}
	log.Info("initialized database")
	g.MountRoute("/api/null/database", db)
	log.Info("mounted database")

	cS := conf.New(db)
	uS := user.New(db)

	g.MountRoute("/api/conf", cS)
	g.MountRoute("/api/u", uS)
	g.Start()
}
