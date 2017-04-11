package main

import (
	"github.com/hackform/governor"
	"github.com/hackform/governor/service/health"
)

func main() {
	g := governor.New(governor.NewConfig())

	hS := health.New()

	g.MountRoute("/api/health", hS)
	g.Start(8080)
}
