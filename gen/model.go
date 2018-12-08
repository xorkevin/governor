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
	"strconv"
	"strings"
	"text/template"
)

const (
	tagFieldName = "model"
	outputTpl    = "gen/model.template"
)

type (
	ASTField struct {
		Ident  string
		GoType string
		Tags   []string
	}

	ModelDef struct {
		Ident      string
		Fields     []ModelField
		PrimaryKey *ModelField
		PKNum      int
	}

	ModelField struct {
		Ident  string
		GoType string
		DBName string
		DBType string
	}

	SQLStrings struct {
		Setup        string
		DBNames      string
		Placeholders string
		Idents       string
		IdentRefs    string
	}

	TemplateData struct {
		Generator  string
		Package    string
		Prefix     string
		TableName  string
		ModelIdent string
		PrimaryKey ModelField
		PKNum      string
		SQL        SQLStrings
	}
)

func argError() {
	log.Fatal("Arguments must be [output_file prefix tablename modelname] which are the output filename, generated function prefix, db table name, and model struct identifier respectively")
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
	if len(args) != 4 {
		argError()
	}
	generatedFilepath := args[0]
	prefix := args[1]
	tableName := args[2]
	modelIdent := args[3]

	fmt.Println(strings.Join([]string{
		"Generating model",
		fmt.Sprintf("Package: %s", gopackage),
		fmt.Sprintf("Source file: %s", gofile),
		fmt.Sprintf("Model ident: %s", modelIdent),
		fmt.Sprintf("Table name: %s", tableName),
	}, "; "))
	fmt.Printf("Working dir: %s\n", workDir)

	modelDef := findStructs(gofile, modelIdent)

	tpl, err := template.ParseFiles(filepath.Join(workDir, outputTpl))
	if err != nil {
		log.Fatal(err)
	}

	genfile, err := os.OpenFile(generatedFilepath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer genfile.Close()
	genFileWriter := bufio.NewWriter(genfile)

	tplData := TemplateData{
		Generator:  "go generate",
		Package:    gopackage,
		Prefix:     prefix,
		TableName:  tableName,
		ModelIdent: modelDef.Ident,
		PrimaryKey: *modelDef.PrimaryKey,
		PKNum:      "$" + strconv.Itoa(modelDef.PKNum),
		SQL:        *modelDef.genSQL(),
	}
	if err := tpl.Execute(genFileWriter, tplData); err != nil {
		log.Fatal(err)
	}
	genFileWriter.Flush()

	fmt.Printf("Generated file: %s\n", generatedFilepath)
}

func findStructs(gofile string, modelIdent string) ModelDef {
	fset := token.NewFileSet()
	root, err := parser.ParseFile(fset, gofile, nil, parser.AllErrors)
	if err != nil {
		log.Fatal(err)
	}
	if root.Decls == nil {
		log.Fatal("No top level declarations")
	}

	modelDef := findStruct(modelIdent, root.Decls)

	modelASTFields := findFields(modelDef, fset)

	modelFields, primaryField, pkNum := parseModelFields(modelASTFields)
	if primaryField == nil {
		log.Fatal("Model does not contain a primary key")
	}
	return ModelDef{
		Ident:      modelIdent,
		Fields:     modelFields,
		PrimaryKey: primaryField,
		PKNum:      pkNum,
	}
}

func findStruct(ident string, decls []ast.Decl) *ast.StructType {
	for _, i := range decls {
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
			if typeSpec.Name.Name == ident {
				return structType
			}
		}
	}

	log.Fatal(ident + " struct not found")
	return nil
}

func findFields(modelDef *ast.StructType, fset *token.FileSet) []ASTField {
	fields := []ASTField{}
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

		goType := bytes.Buffer{}
		if err := printer.Fprint(&goType, fset, field.Type); err != nil {
			log.Fatal(err)
		}

		if len(field.Names) != 1 {
			log.Fatal("Only one field allowed per tag")
		}

		m := ASTField{
			Ident:  field.Names[0].Name,
			GoType: goType.String(),
			Tags:   tags,
		}
		fields = append(fields, m)
	}
	return fields
}

func parseModelFields(astfields []ASTField) ([]ModelField, *ModelField, int) {
	hasPK := false
	pkNum := -1
	var primaryKey ModelField

	fields := []ModelField{}
	for n, i := range astfields {
		if len(i.Tags) != 2 {
			log.Fatal("Model field tag must be dbname,dbtype")
		}
		dbName := i.Tags[0]
		dbType := i.Tags[1]
		f := ModelField{
			Ident:  i.Ident,
			GoType: i.GoType,
			DBName: dbName,
			DBType: dbType,
		}
		if strings.Contains(dbType, "PRIMARY KEY") {
			if hasPK {
				log.Fatal("Model cannot contain two primary keys")
			}
			hasPK = true
			pkNum = n + 1
			primaryKey = f
		}
		fields = append(fields, f)
	}

	return fields, &primaryKey, pkNum
}

func (m *ModelDef) genSQL() *SQLStrings {
	sqlDefs := make([]string, 0, len(m.Fields))
	sqlDBNames := make([]string, 0, len(m.Fields))
	sqlPlaceholders := make([]string, 0, len(m.Fields))
	sqlIdents := make([]string, 0, len(m.Fields))
	sqlIdentRefs := make([]string, 0, len(m.Fields))

	fmt.Println("Detected fields:")
	for n, i := range m.Fields {
		fmt.Printf("- %s %s\n", i.Ident, i.GoType)
		sqlDefs = append(sqlDefs, fmt.Sprintf("%s %s", i.DBName, i.DBType))
		sqlDBNames = append(sqlDBNames, i.DBName)
		sqlPlaceholders = append(sqlPlaceholders, fmt.Sprintf("$%d", n+1))
		sqlIdents = append(sqlIdents, fmt.Sprintf("m.%s", i.Ident))
		sqlIdentRefs = append(sqlIdentRefs, fmt.Sprintf("&m.%s", i.Ident))
	}

	return &SQLStrings{
		Setup:        strings.Join(sqlDefs, ", "),
		DBNames:      strings.Join(sqlDBNames, ", "),
		Placeholders: strings.Join(sqlPlaceholders, ", "),
		Idents:       strings.Join(sqlIdents, ", "),
		IdentRefs:    strings.Join(sqlIdentRefs, ", "),
	}
}
