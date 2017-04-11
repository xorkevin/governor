package main

import (
	"github.com/hackform/governor/staticfs"
)

func main() {
	s := staticfs.New(staticfs.NewConfig())
	s.Start(3000)
}
