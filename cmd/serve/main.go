package main

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/conf"
	"github.com/hackform/governor/service/user"
	_ "github.com/lib/pq"
)

func main() {
	g, err := governor.New(governor.NewConfig())
	if err != nil {
		return
	}

	cS := conf.New()
	uS := user.New()

	g.MountRoute("/api/conf", cS)
	g.MountRoute("/api/u", uS)
	g.Start(8080)
}
