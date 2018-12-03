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
	"reflect"
	"strings"
)

const (
	tagFieldName = "model"
)

type (
	ModelField struct {
		Ident  string
		GoType string
		DBName string
		DBType string
	}
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
	if len(args) != 2 {
		argError()
	}
	modelIdent := args[0]
	tableName := args[1]

	fmt.Println("Generating model")
	fmt.Printf("Package: %s; ", gopackage)
	fmt.Printf("Source file: %s; ", gofile)
	fmt.Printf("Model struct ident: %s; ", modelIdent)
	fmt.Printf("Table name: %s ", tableName)
	fmt.Println()

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

	fields := []ModelField{}
	for _, field := range modelDef.Fields.List {
		if field.Tag == nil {
			continue
		}
		structTag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		tagVal, ok := structTag.Lookup(tagFieldName)
		if !ok {
			continue
		}
		tags := strings.Split(tagVal, ",")
		if len(tags) != 2 {
			log.Fatal("Model field tag must be dbname,dbtype")
		}
		dbName := tags[0]
		dbType := tags[1]

		goType := bytes.Buffer{}
		if err := printer.Fprint(&goType, fset, field.Type); err != nil {
			log.Fatal(err)
		}

		if len(field.Names) != 1 {
			log.Fatal("Only one field allowed per tag")
		}

		fields = append(fields, ModelField{
			Ident:  field.Names[0].Name,
			GoType: goType.String(),
			DBName: dbName,
			DBType: dbType,
		})
	}

	for _, i := range fields {
		fmt.Printf("%s %s %s %s\n", i.Ident, i.GoType, i.DBName, i.DBType)
	}
}
