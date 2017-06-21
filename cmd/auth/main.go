package main

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cache/conf"
	"github.com/hackform/governor/service/conf"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/db/conf"
	"github.com/hackform/governor/service/user"
	"github.com/hackform/governor/service/user/conf"
)

func main() {
	config, err := governor.NewConfig("auth")
	if err != nil {
		fmt.Printf("error reading config: %s\n", err)
		return
	}
	fmt.Println("created new config")

	if err = dbconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("loaded db config defaults")

	if err = cacheconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("loaded cache config defaults")

	if err = userconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("loaded user config defaults")

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

	dbService, err := db.New(config)
	if err != nil {
		log.Errorf("error creating DB: %s\n", err)
		return
	}
	log.Info("initialized database")

	cacheService, err := cache.New(config)
	if err != nil {
		log.Errorf("error creating Cache: %s\n", err)
		return
	}
	log.Info("initialized cache")

	confService := conf.New(dbService)
	log.Info("initialized conf service")

	userService := user.New(config, dbService, cacheService)
	log.Info("initialized user service")

	g.MountRoute("/null/database", dbService)
	log.Info("mounted database")

	g.MountRoute("/null/cache", cacheService)
	log.Info("mounted cache")

	g.MountRoute("/conf", confService)
	log.Info("mounted conf service")

	g.MountRoute("/u", userService)
	log.Info("mounted user service")

	g.Start()
}
