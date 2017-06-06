package main

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/user"
)

func main() {
	config, err := governor.NewConfig("auth")
	if err != nil {
		fmt.Printf("error reading config: %s\n", err)
		return
	}
	fmt.Println("created new config")

	if err = db.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("loaded db config defaults")

	if err = config.Init(); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("config loaded")

	g, err := governor.New(config)
	if err != nil {
		return
	}
	log := g.Logger()
	log.Info("server instance created")

	db, err := db.New(config)
	if err != nil {
		log.Errorf("error creating DB: %s\n", err)
		return
	}
	log.Info("initialized database")

	cS := conf.New(db)
	log.Info("initialized conf service")

	uS := user.New(db)
	log.Info("initialized user service")

	g.MountRoute("/api/conf", cS)
	log.Info("mounted conf service")

	g.MountRoute("/api/u", uS)
	log.Info("mounted user service")

	g.MountRoute("/api/null/database", db)
	log.Info("mounted database")

	g.Start()
}