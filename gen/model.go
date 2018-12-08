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
	tagFieldName   = "model"
	modelOutputTpl = "gen/model.template"
	queryOutputTpl = "gen/model_query_single.template"
)

type (
	ASTField struct {
		Ident  string
		GoType string
		Tags   []string
	}

	ModelDef struct {
		Ident       string
		Fields      []ModelField
		PrimaryKey  ModelField
		QueryFields []QueryField
	}

	ModelField struct {
		Ident  string
		GoType string
		DBName string
		DBType string
		Num    int
	}

	QueryField struct {
		Ident  string
		GoType string
		DBName string
		Num    int
	}

	ModelSQLStrings struct {
		Setup        string
		DBNames      string
		Placeholders string
		Idents       string
		IdentRefs    string
	}

	ModelTemplateData struct {
		Generator  string
		Package    string
		Prefix     string
		TableName  string
		ModelIdent string
		PrimaryKey ModelField
		SQL        ModelSQLStrings
	}

	QuerySQLStrings struct {
		DBNames   string
		IdentRefs string
	}

	QueryTemplateData struct {
		Prefix       string
		TableName    string
		ModelIdent   string
		PrimaryField QueryField
		SQL          QuerySQLStrings
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

	tplmodel, err := template.ParseFiles(filepath.Join(workDir, modelOutputTpl))
	if err != nil {
		log.Fatal(err)
	}

	tplquery, err := template.ParseFiles(filepath.Join(workDir, queryOutputTpl))
	if err != nil {
		log.Fatal(err)
	}

	genfile, err := os.OpenFile(generatedFilepath, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err != nil {
		log.Fatal(err)
	}
	defer genfile.Close()
	genFileWriter := bufio.NewWriter(genfile)

	tplData := ModelTemplateData{
		Generator:  "go generate",
		Package:    gopackage,
		Prefix:     prefix,
		TableName:  tableName,
		ModelIdent: modelDef.Ident,
		PrimaryKey: modelDef.PrimaryKey,
		SQL:        modelDef.genModelSQL(),
	}
	if err := tplmodel.Execute(genFileWriter, tplData); err != nil {
		log.Fatal(err)
	}

	querySQLStrings := modelDef.genQuerySQL()
	for _, i := range modelDef.QueryFields {
		tplData := QueryTemplateData{
			Prefix:       prefix,
			TableName:    tableName,
			ModelIdent:   modelDef.Ident,
			PrimaryField: i,
			SQL:          querySQLStrings,
		}
		if err := tplquery.Execute(genFileWriter, tplData); err != nil {
			log.Fatal(err)
		}
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

	modelFields, primaryField, queryFields := parseModelFields(modelASTFields)

	return ModelDef{
		Ident:       modelIdent,
		Fields:      modelFields,
		PrimaryKey:  primaryField,
		QueryFields: queryFields,
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

func parseModelFields(astfields []ASTField) ([]ModelField, ModelField, []QueryField) {
	hasPK := false
	var primaryKey ModelField
	queryFields := []QueryField{}

	fields := []ModelField{}
	for n, i := range astfields {
		if len(i.Tags) < 2 || len(i.Tags) > 3 {
			log.Fatal("Model field tag must be dbname,dbtype,flag(optional)")
		}
		dbName := i.Tags[0]
		dbType := i.Tags[1]
		if len(dbName) == 0 {
			log.Fatal(i.Ident + " dbname not set")
		}
		if len(dbType) == 0 {
			log.Fatal(i.Ident + " dbtype not set")
		}
		f := ModelField{
			Ident:  i.Ident,
			GoType: i.GoType,
			DBName: dbName,
			DBType: dbType,
			Num:    n + 1,
		}
		if strings.Contains(dbType, "PRIMARY KEY") {
			if hasPK {
				log.Fatal("Model cannot contain two primary keys")
			}
			hasPK = true
			primaryKey = f
		}
		fields = append(fields, f)
		if len(i.Tags) == 3 && parseFlag(i.Tags[2]) == flagGet {
			queryFields = append(queryFields, QueryField{
				Ident:  i.Ident,
				GoType: i.GoType,
				DBName: dbName,
				Num:    n + 1,
			})
		}
	}

	if !hasPK {
		log.Fatal("Model does not contain a primary key")
	}

	return fields, primaryKey, queryFields
}

//func parseQueryFields(astfields []ASTField) ([]QueryField, []QueryField) {
//	hasQF := false
//	queryFields := []QueryField{}
//
//	fields := []QueryField{}
//	for n, i := range astfields {
//		if len(i.Tags) < 1 || len(i.Tags) > 2 {
//			log.Fatal("Field tag must be dbname,flag(optional)")
//		}
//		dbName := i.Tags[0]
//		f := QueryField{
//			Ident:  i.Ident,
//			GoType: i.GoType,
//			DBName: dbName,
//			Num:    n + 1,
//		}
//		fields = append(fields, f)
//		if len(i.Tags) == 2 && parseFlag(i.Tags[1]) == flagGet {
//			hasQF = true
//			queryFields = append(queryFields, f)
//		}
//	}
//
//	if !hasQF {
//		log.Fatal("Query does not contain a query field")
//	}
//
//	return fields, queryFields
//}

const (
	flagGet = iota
)

func parseFlag(flag string) int {
	switch flag {
	case "get":
		return flagGet
	default:
		log.Fatal("Illegal flag " + flag)
	}
	return -1
}

func (m *ModelDef) genModelSQL() ModelSQLStrings {
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

	return ModelSQLStrings{
		Setup:        strings.Join(sqlDefs, ", "),
		DBNames:      strings.Join(sqlDBNames, ", "),
		Placeholders: strings.Join(sqlPlaceholders, ", "),
		Idents:       strings.Join(sqlIdents, ", "),
		IdentRefs:    strings.Join(sqlIdentRefs, ", "),
	}
}

func (m *ModelDef) genQuerySQL() QuerySQLStrings {
	sqlDBNames := make([]string, 0, len(m.Fields))
	sqlIdentRefs := make([]string, 0, len(m.Fields))

	fmt.Println("Detected fields:")
	for _, i := range m.Fields {
		fmt.Printf("- %s %s\n", i.Ident, i.GoType)
		sqlDBNames = append(sqlDBNames, i.DBName)
		sqlIdentRefs = append(sqlIdentRefs, fmt.Sprintf("&m.%s", i.Ident))
	}

	return QuerySQLStrings{
		DBNames:   strings.Join(sqlDBNames, ", "),
		IdentRefs: strings.Join(sqlIdentRefs, ", "),
	}
}
