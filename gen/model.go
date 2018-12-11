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
	modelTagName          = "model"
	queryTagName          = "query"
	modelOutputTpl        = "gen/model.template"
	queryOutputTpl        = "gen/model_query_single.template"
	queryGroupOutputTpl   = "gen/model_query_group.template"
	queryGroupEqOutputTpl = "gen/model_query_groupeq.template"
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
		PrimaryKey ModelField
	}

	QueryDef struct {
		Ident       string
		Fields      []QueryField
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
		Mode   int
		Cond   ModelField
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
	if len(args) < 4 {
		argError()
	}
	generatedFilepath := args[0]
	prefix := args[1]
	tableName := args[2]
	modelIdent := args[3]
	queryIdents := args[4:]

	fmt.Println(strings.Join([]string{
		"Generating model",
		fmt.Sprintf("Package: %s", gopackage),
		fmt.Sprintf("Source file: %s", gofile),
		fmt.Sprintf("Table name: %s", tableName),
		fmt.Sprintf("Model ident: %s", modelIdent),
		fmt.Sprintf("Additional queries: %s", strings.Join(queryIdents, ", ")),
	}, "; "))
	fmt.Printf("Working dir: %s\n", workDir)

	modelDef, queryDefs := parseDefinitions(gofile, modelIdent, queryIdents)

	tplmodel, err := template.ParseFiles(filepath.Join(workDir, modelOutputTpl))
	if err != nil {
		log.Fatal(err)
	}

	tplquery, err := template.ParseFiles(filepath.Join(workDir, queryOutputTpl))
	if err != nil {
		log.Fatal(err)
	}

	tplquerygroup, err := template.ParseFiles(filepath.Join(workDir, queryGroupOutputTpl))
	if err != nil {
		log.Fatal(err)
	}

	tplquerygroupeq, err := template.ParseFiles(filepath.Join(workDir, queryGroupEqOutputTpl))
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

	fmt.Println("Detected model fields:")
	for _, i := range modelDef.Fields {
		fmt.Printf("- %s %s\n", i.Ident, i.GoType)
	}

	for _, queryDef := range queryDefs {
		fmt.Println("Detected query " + queryDef.Ident + " fields:")
		for _, i := range queryDef.Fields {
			fmt.Printf("- %s %s\n", i.Ident, i.GoType)
		}
		querySQLStrings := queryDef.genQuerySQL()
		for _, i := range queryDef.QueryFields {
			tplData := QueryTemplateData{
				Prefix:       prefix,
				TableName:    tableName,
				ModelIdent:   queryDef.Ident,
				PrimaryField: i,
				SQL:          querySQLStrings,
			}
			switch i.Mode {
			case flagGet:
				if err := tplquery.Execute(genFileWriter, tplData); err != nil {
					log.Fatal(err)
				}
			case flagGetGroup:
				if err := tplquerygroup.Execute(genFileWriter, tplData); err != nil {
					log.Fatal(err)
				}
			case flagGetGroupEq:
				if err := tplquerygroupeq.Execute(genFileWriter, tplData); err != nil {
					log.Fatal(err)
				}
			}
		}
	}

	genFileWriter.Flush()

	fmt.Printf("Generated file: %s\n", generatedFilepath)
}

func parseDefinitions(gofile string, modelIdent string, queryIdents []string) (ModelDef, []QueryDef) {
	fset := token.NewFileSet()
	root, err := parser.ParseFile(fset, gofile, nil, parser.AllErrors)
	if err != nil {
		log.Fatal(err)
	}
	if root.Decls == nil {
		log.Fatal("No top level declarations")
	}

	modelFields, primaryField, seenFields := parseModelFields(findFields(modelTagName, findStruct(modelIdent, root.Decls), fset))

	queryDefs := []QueryDef{}
	for _, ident := range queryIdents {
		fields, queries := parseQueryFields(findFields(queryTagName, findStruct(ident, root.Decls), fset), seenFields)
		queryDefs = append(queryDefs, QueryDef{
			Ident:       ident,
			Fields:      fields,
			QueryFields: queries,
		})
	}

	return ModelDef{
		Ident:      modelIdent,
		Fields:     modelFields,
		PrimaryKey: primaryField,
	}, queryDefs
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

func findFields(tagName string, modelDef *ast.StructType, fset *token.FileSet) []ASTField {
	fields := []ASTField{}
	for _, field := range modelDef.Fields.List {
		if field.Tag == nil {
			continue
		}
		structTag := reflect.StructTag(strings.Trim(field.Tag.Value, "`"))
		tagVal, ok := structTag.Lookup(tagName)
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

func parseModelFields(astfields []ASTField) ([]ModelField, ModelField, map[string]ModelField) {
	hasPK := false
	var primaryKey ModelField

	seenFields := map[string]ModelField{}

	fields := []ModelField{}
	for n, i := range astfields {
		if len(i.Tags) != 2 {
			log.Fatal("Model field tag must be dbname,dbtype")
		}
		dbName := i.Tags[0]
		dbType := i.Tags[1]
		if len(dbName) == 0 {
			log.Fatal(i.Ident + " dbname not set")
		}
		if len(dbType) == 0 {
			log.Fatal(i.Ident + " dbtype not set")
		}
		if _, ok := seenFields[dbName]; ok {
			log.Fatal("Duplicate field " + dbName)
		}
		f := ModelField{
			Ident:  i.Ident,
			GoType: i.GoType,
			DBName: dbName,
			DBType: dbType,
			Num:    n + 1,
		}
		seenFields[dbName] = f
		if strings.Contains(dbType, "PRIMARY KEY") {
			if hasPK {
				log.Fatal("Model cannot contain two primary keys")
			}
			hasPK = true
			primaryKey = f
		}
		fields = append(fields, f)
	}

	if !hasPK {
		log.Fatal("Model does not contain a primary key")
	}

	return fields, primaryKey, seenFields
}

func parseQueryFields(astfields []ASTField, seenFields map[string]ModelField) ([]QueryField, []QueryField) {
	hasQF := false
	queryFields := []QueryField{}

	fields := []QueryField{}
	for n, i := range astfields {
		if len(i.Tags) < 1 || len(i.Tags) > 3 {
			log.Fatal("Field tag must be dbname,flag(optional),eqcond(optional)")
		}
		dbName := i.Tags[0]
		if modelField, ok := seenFields[dbName]; !ok || i.GoType != modelField.GoType {
			log.Fatal("Field " + dbName + " with type " + i.GoType + " does not exist on model")
		}
		f := QueryField{
			Ident:  i.Ident,
			GoType: i.GoType,
			DBName: dbName,
			Num:    n + 1,
		}
		fields = append(fields, f)
		if len(i.Tags) > 1 {
			hasQF = true
			tagflag := parseFlag(i.Tags[1])
			f.Mode = tagflag
			switch tagflag {
			case flagGetGroupEq:
				if len(i.Tags) < 3 {
					log.Fatal("Must provide field for eq condition for field " + i.Ident)
				}
				cond := i.Tags[2]
				if modelField, ok := seenFields[cond]; ok {
					f.Cond = modelField
				} else {
					log.Fatal("Invalid eq condition field for field " + i.Ident)
				}
			}
			queryFields = append(queryFields, f)
		}
	}

	if !hasQF {
		log.Fatal("Query does not contain a query field")
	}

	return fields, queryFields
}

const (
	flagGet = iota
	flagGetGroup
	flagGetGroupEq
)

func parseFlag(flag string) int {
	switch flag {
	case "get":
		return flagGet
	case "getgroup":
		return flagGetGroup
	case "getgroupeq":
		return flagGetGroupEq
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

	for n, i := range m.Fields {
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

	for _, i := range m.Fields {
		sqlDBNames = append(sqlDBNames, i.DBName)
		sqlIdentRefs = append(sqlIdentRefs, fmt.Sprintf("&m.%s", i.Ident))
	}

	return QuerySQLStrings{
		DBNames:   strings.Join(sqlDBNames, ", "),
		IdentRefs: strings.Join(sqlIdentRefs, ", "),
	}
}

func (q *QueryDef) genQuerySQL() QuerySQLStrings {
	sqlDBNames := make([]string, 0, len(q.Fields))
	sqlIdentRefs := make([]string, 0, len(q.Fields))

	for _, i := range q.Fields {
		sqlDBNames = append(sqlDBNames, i.DBName)
		sqlIdentRefs = append(sqlIdentRefs, fmt.Sprintf("&m.%s", i.Ident))
	}

	return QuerySQLStrings{
		DBNames:   strings.Join(sqlDBNames, ", "),
		IdentRefs: strings.Join(sqlIdentRefs, ", "),
	}
}
