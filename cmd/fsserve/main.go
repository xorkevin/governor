package main

import (
	"fmt"
	"github.com/hackform/governor/staticfs"
)

func main() {
	config, err := staticfs.NewConfig()
	if err != nil {
		fmt.Printf("error reading config: %s\n", err)
		return
	}
	s := staticfs.New(config)
	s.Start()
}
