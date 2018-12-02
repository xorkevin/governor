// +build ignore

package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
)

func argError() {
	log.Fatal("Arguments must be [Repo Model] which are the repository interface and model struct respectively")
}

func main() {
	gopackage := os.Getenv("GOPACKAGE")
	if len(gopackage) == 0 {
		log.Fatal("Environment variable GOPACKAGE not provided by go generate")
	}
	gofile := os.Getenv("GOFILE")
	if len(gofile) == 0 {
		log.Fatal("Environment variable GOPACKAGE not provided by go generate")
	}

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
	if len(args) != 1 {
		argError()
	}
	modelIdent := args[0]

	fmt.Println("Package: ", gopackage)
	fmt.Println("Source file: ", gofile)
	fmt.Println("Model struct identifier: ", modelIdent)

	fset := token.NewFileSet()
	root, err := parser.ParseFile(fset, gofile, nil, parser.AllErrors)
	if err != nil {
		log.Fatal(err)
	}
	if root.Decls == nil {
		log.Fatal("No top level declarations")
	}

	var modelDef *ast.StructType
	for _, i := range root.Decls {
		if typeDecl, ok := i.(*ast.GenDecl); ok && typeDecl.Tok == token.TYPE {
			for _, j := range typeDecl.Specs {
				if typeSpec, ok := j.(*ast.TypeSpec); ok {
					if structType, ok := typeSpec.Type.(*ast.StructType); ok && !structType.Incomplete {
						if typeSpec.Name.Name == modelIdent {
							modelDef = structType
							break
						}
					}
				}
			}
		}
	}

	if modelDef == nil {
		log.Fatal("Model struct not found")
	}

	for _, field := range modelDef.Fields.List {
		typeName := bytes.Buffer{}

		if err := printer.Fprint(&typeName, fset, field.Type); err != nil {
			log.Fatal(err)
		}
		tag := ""
		if field.Tag != nil {
			tag = field.Tag.Value
		}
		for _, name := range field.Names {
			fmt.Printf("%s ", name.Name)
		}
		fmt.Printf("%s %s\n", typeName.String(), tag)
	}
}
