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
	"github.com/hackform/governor/service/image"
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
	"github.com/hackform/governor/service/user/model"
	"github.com/hackform/governor/service/user/role/model"
)

var (
	// GitHash is the git hash to be passed in at compile time
	GitHash string
)

func main() {
	config, err := governor.NewConfig("config", GitHash)
	governor.Must(err)

	fmt.Println("created new config")
	fmt.Println("loading config defaults:")

	governor.Must(dbconf.Conf(&config))
	fmt.Println("- db")

	governor.Must(cacheconf.Conf(&config))
	fmt.Println("- cache")

	governor.Must(objstoreconf.Conf(&config))
	fmt.Println("- objstore")

	governor.Must(mailconf.Conf(&config))
	fmt.Println("- mail")

	governor.Must(gateconf.Conf(&config))
	fmt.Println("- gate")

	governor.Must(userconf.Conf(&config))
	fmt.Println("- user")

	governor.Must(profileconf.Conf(&config))
	fmt.Println("- profile")

	governor.Must(postconf.Conf(&config))
	fmt.Println("- post")

	governor.Must(config.Init())
	fmt.Println("config loaded")

	g, err := governor.New(config)
	governor.Must(err)

	dbService, err := db.New(config, g.Logger())
	governor.Must(err)

	cacheService, err := cache.New(config, g.Logger())
	governor.Must(err)

	objstoreService, err := objstore.New(config, g.Logger())
	governor.Must(err)

	templateService, err := template.New(config, g.Logger())
	governor.Must(err)

	mailService := mail.New(config, g.Logger(), templateService)

	gateService := gate.New(config, g.Logger())

	imageService := image.New(config, g.Logger())

	cacheControlService := cachecontrol.New(config, g.Logger())

	roleModelService := rolemodel.New(config, g.Logger(), dbService)
	userModelService := usermodel.New(config, g.Logger(), dbService, roleModelService)
	userService := user.New(config, g.Logger(), userModelService, roleModelService, cacheService, mailService, gateService, cacheControlService)

	profileService := profile.New(config, g.Logger(), dbService, objstoreService, gateService, imageService, cacheControlService)
	userService.RegisterHook(profileService)

	governor.Must(g.MountRoute("/null/database", dbService))
	governor.Must(g.MountRoute("/null/cache", cacheService))
	governor.Must(g.MountRoute("/null/objstore", objstoreService))
	governor.Must(g.MountRoute("/null/mail", mailService))
	governor.Must(g.MountRoute("/conf", conf.New(g.Logger(), dbService)))
	governor.Must(g.MountRoute("/u", userService))
	governor.Must(g.MountRoute("/profile", profileService))
	governor.Must(g.MountRoute("/post", post.New(config, g.Logger(), dbService, cacheService, gateService, cacheControlService)))

	governor.Must(g.Start())
}
