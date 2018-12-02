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
	"strings"
)

const (
	tagFieldName = "model"
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

	fmt.Println("Generating model")
	fmt.Printf("Package: %s; Source file: %s; Model struct ident: %s\n", gopackage, gofile, modelIdent)

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
		typeDecl, ok := i.(*ast.GenDecl)
		if !ok || typeDecl.Tok != token.TYPE {
			continue
		}
		for _, j := range typeDecl.Specs {
			typeSpec, ok := j.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok || structType.Incomplete {
				continue
			}
			if typeSpec.Name.Name == modelIdent {
				modelDef = structType
				break
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
			tags := strings.Fields(strings.Trim(field.Tag.Value, "`"))
			for _, i := range tags {
				tagFields := strings.Split(i, ":")
				if len(tagFields) != 2 || tagFields[0] != tagFieldName {
					continue
				}
				tagVal := strings.Trim(tagFields[1], "\"")
				if len(tagVal) == 0 {
					continue
				}
				tag = tagVal
				break
			}
		}
		if len(tag) == 0 {
			continue
		}
		for _, name := range field.Names {
			fmt.Printf("%s ", name.Name)
		}
		fmt.Printf("%s %s\n", typeName.String(), tag)
	}
}
