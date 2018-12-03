// +build ignore

package main

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"text/template"
)

const (
	tagFieldName = "model"
	outputTpl    = "gen/model.template"
)

type (
	ModelField struct {
		Ident  string
		GoType string
		DBName string
		DBType string
	}

	TemplateData struct {
		Generator  string
		Package    string
		ModelIdent string
		TableName  string
		PrimaryKey ModelField
		SQLSetup   string
	}
)

func argError() {
	log.Fatal("Arguments must be [output_file modelname tablename] which are the output filename, model struct identifier, and db table name respectively")
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
	workDir := os.Getenv("PWD")
	if len(workDir) == 0 {
		log.Fatal("Environment variable PWD not set")
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
	if len(args) != 3 {
		argError()
	}
	generatedFilepath := args[0]
	modelIdent := args[1]
	tableName := args[2]

	fmt.Println(strings.Join([]string{
		"Generating model",
		fmt.Sprintf("Package: %s", gopackage),
		fmt.Sprintf("Source file: %s", gofile),
		fmt.Sprintf("Model ident: %s", modelIdent),
		fmt.Sprintf("Table name: %s", tableName),
	}, "; "))
	fmt.Printf("Working dir: %s\n", workDir)

	tpl, err := template.ParseFiles(filepath.Join(workDir, outputTpl))
	if err != nil {
		log.Fatal(err)
	}

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

		m := ModelField{
			Ident:  field.Names[0].Name,
			GoType: goType.String(),
			DBName: dbName,
			DBType: dbType,
		}
		fields = append(fields, m)
	}

	hasPK := false
	var primaryKey ModelField

	sqlDefFields := make([]string, 0, len(fields))

	fmt.Println("Detected fields:")
	for _, i := range fields {
		fmt.Printf("- %s %s\n", i.Ident, i.GoType)
		if strings.Contains(i.DBType, "PRIMARY KEY") {
			if hasPK {
				log.Fatal("Model cannot contain two primary keys")
			}
			hasPK = true
			primaryKey = i
		}
		sqlDefFields = append(sqlDefFields, fmt.Sprintf("%s %s", i.DBName, i.DBType))
	}

	if !hasPK {
		log.Fatal("Model does not contain a primary key")
	}

	sqlSetup := strings.Join(sqlDefFields, ", ")

	genfile, err := os.OpenFile(generatedFilepath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer genfile.Close()
	genFileWriter := bufio.NewWriter(genfile)

	tplData := TemplateData{
		Generator:  "go generate",
		Package:    gopackage,
		ModelIdent: modelIdent,
		TableName:  tableName,
		PrimaryKey: primaryKey,
		SQLSetup:   sqlSetup,
	}
	if err := tpl.Execute(genFileWriter, tplData); err != nil {
		log.Fatal(err)
	}
	genFileWriter.Flush()

	fmt.Printf("Generated file: %s\n", generatedFilepath)
}
