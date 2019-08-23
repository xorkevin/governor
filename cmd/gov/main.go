package main

import (
	"fmt"
	"xorkevin.dev/governor"
	"xorkevin.dev/governor/service/barcode"
	"xorkevin.dev/governor/service/cache"
	"xorkevin.dev/governor/service/cache/conf"
	"xorkevin.dev/governor/service/cachecontrol"
	"xorkevin.dev/governor/service/conf"
	"xorkevin.dev/governor/service/conf/model"
	"xorkevin.dev/governor/service/courier"
	"xorkevin.dev/governor/service/courier/model"
	"xorkevin.dev/governor/service/db"
	"xorkevin.dev/governor/service/db/conf"
	"xorkevin.dev/governor/service/fileloader"
	"xorkevin.dev/governor/service/image"
	"xorkevin.dev/governor/service/mail"
	"xorkevin.dev/governor/service/mail/conf"
	"xorkevin.dev/governor/service/msgqueue"
	"xorkevin.dev/governor/service/msgqueue/conf"
	"xorkevin.dev/governor/service/objstore"
	"xorkevin.dev/governor/service/objstore/conf"
	"xorkevin.dev/governor/service/profile"
	"xorkevin.dev/governor/service/profile/conf"
	"xorkevin.dev/governor/service/profile/model"
	"xorkevin.dev/governor/service/template"
	"xorkevin.dev/governor/service/user"
	"xorkevin.dev/governor/service/user/conf"
	"xorkevin.dev/governor/service/user/gate"
	"xorkevin.dev/governor/service/user/gate/conf"
	"xorkevin.dev/governor/service/user/model"
	"xorkevin.dev/governor/service/user/role/model"
	"xorkevin.dev/governor/service/websocket"
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

	governor.Must(msgqueueconf.Conf(&config))
	fmt.Println("- msgqueue")

	governor.Must(mailconf.Conf(&config))
	fmt.Println("- mail")

	governor.Must(gateconf.Conf(&config))
	fmt.Println("- gate")

	governor.Must(userconf.Conf(&config))
	fmt.Println("- user")

	governor.Must(profileconf.Conf(&config))
	fmt.Println("- profile")

	governor.Must(config.Init())
	fmt.Println("config loaded")

	l := governor.NewLogger(config)

	g, err := governor.New(config, l)
	governor.Must(err)

	dbService, err := db.New(config, l)
	governor.Must(err)

	cacheService, err := cache.New(config, l)
	governor.Must(err)

	objstoreService, err := objstore.New(config, l)
	governor.Must(err)

	queueService, err := msgqueue.New(config, l)
	governor.Must(err)

	templateService, err := template.New(config, l)
	governor.Must(err)

	mailService, err := mail.New(config, l, templateService, queueService)
	governor.Must(err)

	gateService := gate.New(config, l)

	imageService := image.New(config, l)

	cacheControlService := cachecontrol.New(config, l)

	confModelService := confmodel.New(config, l, dbService)
	confService := conf.New(config, l, confModelService)

	roleModelService := rolemodel.New(config, l, dbService)
	userModelService := usermodel.New(config, l, dbService, roleModelService)
	userService := user.New(config, l, userModelService, roleModelService, cacheService, mailService, gateService, cacheControlService)

	profileModelService := profilemodel.New(config, l, dbService)
	profileService, err := profile.New(config, l, profileModelService, objstoreService, gateService, imageService, cacheControlService)
	governor.Must(err)
	userService.RegisterHook(profileService)

	barcodeService := barcode.New(config, l)
	courierModelService := couriermodel.New(config, l, dbService)
	courierService, err := courier.New(config, l, courierModelService, objstoreService, barcodeService, cacheService, gateService, cacheControlService)
	governor.Must(err)

	fileloader.New(config, l)
	websocket.New(config, l)

	governor.Must(g.MountRoute("/null/database", dbService))
	governor.Must(g.MountRoute("/null/cache", cacheService))
	governor.Must(g.MountRoute("/null/objstore", objstoreService))
	governor.Must(g.MountRoute("/conf", confService))
	governor.Must(g.MountRoute("/u", userService))
	governor.Must(g.MountRoute("/profile", profileService))
	governor.Must(g.MountRoute("/courier", courierService))

	governor.Must(g.Start())
}
