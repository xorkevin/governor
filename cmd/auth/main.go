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

	cS := conf.New()
	uS := user.New()

	g.MountRoute("/api/conf", cS)
	g.MountRoute("/api/u", uS)
	g.Start()
}
