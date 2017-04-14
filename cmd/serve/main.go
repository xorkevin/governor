package main

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/health"
	"github.com/hackform/governor/service/user"
)

func main() {
	g := governor.New(governor.NewConfig())

	hS := health.New()
	uS := user.New()

	g.MountRoute("/api/health", hS)
	g.MountRoute("/api/u", uS)
	g.Start(8080)
}
