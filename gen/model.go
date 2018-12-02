// +build ignore

package main

import (
	"fmt"
	"log"
	"os"
)

func argError() {
	log.Fatal("Arguments must be [Repo Model] which are the repository interface and model struct respectively")
}

func main() {
	fmt.Println(os.Getenv("GOPACKAGE"))
	fmt.Println(os.Getenv("GOFILE"))

	argStart := -1
	for n, i := range os.Args {
		if i == "--" {
			argStart = n
			break
		}
	}
	if argStart < 0 {
		argError()
	}

	args := os.Args[argStart+1:]
	if len(args) != 2 {
		argError()
	}
}
