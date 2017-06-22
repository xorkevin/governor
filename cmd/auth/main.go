package main

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cache/conf"
	"github.com/hackform/governor/service/conf"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/db/conf"
	"github.com/hackform/governor/service/mail"
	"github.com/hackform/governor/service/mail/conf"
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
	fmt.Println("loading config defaults:")

	if err = dbconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- db")

	if err = cacheconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- cache")

	if err = mailconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- mail")

	if err = userconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- user")

	if err = config.Init(); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("config loaded")

	g, err := governor.New(config)
	if err != nil {
		return
	}

	dbService, err := db.New(config, g.Logger())
	if err != nil {
		return
	}

	cacheService, err := cache.New(config, g.Logger())
	if err != nil {
		return
	}

	mailService := mail.New(config, g.Logger())

	confService := conf.New(g.Logger(), dbService)

	userService := user.New(config, g.Logger(), dbService, cacheService)

	g.MountRoute("/null/database", dbService)

	g.MountRoute("/null/cache", cacheService)

	g.MountRoute("/null/mail", mailService)

	g.MountRoute("/conf", confService)

	g.MountRoute("/u", userService)

	g.Start()
}
