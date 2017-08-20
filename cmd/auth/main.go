package main

import (
	"fmt"
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/cache"
	"github.com/hackform/governor/service/cache/conf"
	"github.com/hackform/governor/service/cachecontrol"
	"github.com/hackform/governor/service/conf"
	"github.com/hackform/governor/service/db"
	"github.com/hackform/governor/service/db/conf"
	"github.com/hackform/governor/service/mail"
	"github.com/hackform/governor/service/mail/conf"
	"github.com/hackform/governor/service/objstore"
	"github.com/hackform/governor/service/objstore/conf"
	"github.com/hackform/governor/service/post"
	"github.com/hackform/governor/service/post/conf"
	"github.com/hackform/governor/service/profile"
	"github.com/hackform/governor/service/profile/conf"
	"github.com/hackform/governor/service/template"
	"github.com/hackform/governor/service/user"
	"github.com/hackform/governor/service/user/conf"
	"github.com/hackform/governor/service/user/gate"
	"github.com/hackform/governor/service/user/gate/conf"
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

	if err = objstoreconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- objstore")

	if err = mailconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- mail")

	if err = gateconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- gate")

	if err = userconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- user")

	if err = profileconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- profile")

	if err = postconf.Conf(&config); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("- post")

	if err = config.Init(); err != nil {
		fmt.Printf(err.Error())
		return
	}
	fmt.Println("config loaded")

	g, err := governor.New(config)
	if err != nil {
		fmt.Println(err)
		return
	}

	dbService, err := db.New(config, g.Logger())
	if err != nil {
		fmt.Println(err)
		return
	}

	cacheService, err := cache.New(config, g.Logger())
	if err != nil {
		fmt.Println(err)
		return
	}

	objstoreService, err := objstore.New(config, g.Logger())
	if err != nil {
		fmt.Println(err)
		return
	}

	templateService, err := template.New(config, g.Logger())
	if err != nil {
		fmt.Println(err)
	}

	mailService := mail.New(config, g.Logger())

	gateService := gate.New(config, g.Logger())

	cacheControlService := cachecontrol.New(config, g.Logger())

	if err := g.MountRoute("/null/database", dbService); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/null/cache", cacheService); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/null/objstore", objstoreService); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/null/mail", mailService); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/conf", conf.New(g.Logger(), dbService)); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/u", user.New(config, g.Logger(), dbService, cacheService, mailService, gateService, cacheControlService)); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/profile", profile.New(config, g.Logger(), dbService, cacheService, objstoreService, gateService, cacheControlService)); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.MountRoute("/post", post.New(config, g.Logger(), dbService, cacheService, gateService, cacheControlService)); err != nil {
		fmt.Println(err)
		return
	}

	if err := g.Start(); err != nil {
		fmt.Println(err)
		return
	}
}
