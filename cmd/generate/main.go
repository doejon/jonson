package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/doc"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/doejon/jonson"
)

var (
	fpath          = "."
	jonsonPath     = "github.com/doejon/jonson"
	apiNameMatcher = regexp.MustCompile(`^(.+)V([0-9]+)$`)
	apiTypeMatcher = regexp.MustCompile(`(^|\n)@generate($|\n)`)
)

func init() {
	flag.StringVar(&fpath, "path", fpath, "filepath to scan")
	flag.StringVar(&jonsonPath, "jonson", jonsonPath, "path to jonson library")
	flag.Parse()
}

func inList(s string, list []string) bool {
	for _, cmp := range list {
		if s == cmp {
			return true
		}
	}
	return false
}

// file storing internal api calls
const fNameProcedure = "jonson.procedure-calls.gen.go"

// file storing provider types
const fNameProvider = "jonson.providers.gen.go"

// some file names need to be ignored
var ignoredFileNames = map[string]struct{}{
	fNameProcedure: {},
	fNameProvider:  {},
}

// readPackage reads package.
// ast.Package has been deprecated, however
// none of the other libraries did migrate so far either.
func readPackage() (*ast.Package, *token.FileSet, error) {
	stat, err := os.Stat(fpath)
	if err != nil {
		return nil, nil, err
	}
	if !stat.IsDir() {
		return nil, nil, errors.New("valid path file expected")
	}

	// parse ast
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, fpath, func(fi os.FileInfo) bool {
		_, ok := ignoredFileNames[fi.Name()]
		return !ok
	}, parser.ParseComments)
	if err != nil {
		return nil, nil, err
	}

	// search for type declarations
	if len(pkgs) != 1 {
		return nil, nil, fmt.Errorf("exactly 1 package expected, got: %d in path %s", len(pkgs), fpath)
	}
	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
	}

	return pkg, fset, nil
}

func writeFile(name string, content []byte) error {
	outputFilename := filepath.Join(fpath, name)
	return os.WriteFile(outputFilename, content, 0644)
}

func prependHeader(wtr *bytes.Buffer, pkgName string, imports ...string) []byte {
	for i, v := range imports {
		imports[i] = "\"" + v + "\""
	}
	// file header
	fileContent := wtr.String()
	wtr.Reset()

	fileContent = fmt.Sprintf(
		`// code generated by jonson-generate; DO NOT EDIT.
		
package `+pkgName+`

import (
	`+strings.Join(imports, "\n")+`
)

`) + fileContent
	return []byte(fileContent)
}

// writeProcedureFile writes the procedure file
func writeProcedureFile(fset *token.FileSet, pkgName string, listMethods []*ast.FuncDecl, structs []*ast.Object) error {
	wtr := bytes.NewBuffer([]byte{})

	findObject := func(ident *ast.Ident) *ast.Object {
		if ident == nil {
			return nil
		}
		if ident.Obj != nil {
			return ident.Obj
		}
		for _, v := range structs {
			if ident.Name == v.Name {
				return v
			}
		}
		return nil
	}

	needsJonsonImport := false

	extractParam := func(lastArg *ast.Field, decl *ast.TypeSpec, ident *ast.Ident, stru *ast.StructType) string {
		lastArgName := ""
		if len(lastArg.Names) > 0 {
			lastArgName = lastArg.Names[0].Name
		}

		// in case the last param is called 'params', we enforce the use of params
		// otherwise the last argument could also be something else than a param.
		expectParam := lastArgName == "params"

		var err error
		if stru.Fields == nil || len(stru.Fields.List) <= 0 {
			err = fmt.Errorf("missing jonson.Params field in struct %s", decl.Name.String())
		} else if p, ok := stru.Fields.List[0].Type.(*ast.SelectorExpr); ok {
			if p.X.(*ast.Ident).Name == "jonson" && p.Sel.Name == "Params" {
				return ident.Name
			}
		} else {
			err = fmt.Errorf("jonson.Params field in struct %s, should be first field", decl.Name.String())
		}
		if err != nil && expectParam {
			panic(err)
		}
		return ""
	}

	// walk methods
	for _, object := range listMethods {
		pos := fset.Position(object.Pos())

		if object.Recv == nil {
			continue
		}
		if len(object.Recv.List) <= 0 {
			continue
		}

		objStarExp, ok := object.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		objIdent, ok := objStarExp.X.(*ast.Ident)
		if !ok {
			continue
		}

		// only take those into account that equal package name
		if firstToLower(objIdent.Name) != pkgName {
			continue
		}
		systemName := jonson.ToKebabCase(objIdent.Name)

		// input
		var paramType string

		// by default, all rpc calls use post.
		// However, in case the developer wants to force certain http methods,
		// we will find them in the function signature.
		rpcHttpMethod := getRpcHttpMethod(object)

		if len(object.Type.Params.List) > 0 {
			lastArg := object.Type.Params.List[len(object.Type.Params.List)-1]
			starExpr, ok := lastArg.Type.(*ast.StarExpr)
			if ok {
				ident, ok := starExpr.X.(*ast.Ident)
				identObj := findObject(ident)
				if ok && identObj != nil {
					if decl, ok := identObj.Decl.(*ast.TypeSpec); ok {
						if stru, ok := decl.Type.(*ast.StructType); ok {
							paramType = extractParam(lastArg, decl, ident, stru)
						}
					}
				}
			}
		}

		var params, parArg string
		if paramType != "" {
			params = ", p *" + paramType
			parArg = "p"
		} else {
			parArg = "nil"
		}

		// output
		var resultType string
		if l := len(object.Type.Results.List); l == 2 {
			resultType = object.Type.Results.List[0].Type.(*ast.StarExpr).X.(*ast.Ident).Name
		} else if l != 1 {
			panic(errors.New("unexpected results count"))
		}

		var result, vAssign, errRet, valRet, nilRet string
		if resultType != "" {
			result = "(*" + resultType + ", error)"
			vAssign = "v, err"
			errRet = "nil, err"
			valRet = `if v != nil {
		return v.(*` + resultType + `), nil
	}`
			nilRet = "nil, nil"
		} else {
			result = "error"
			vAssign = "_, err"
			errRet = "err"
			nilRet = "nil"
		}

		// comment line
		needsJonsonImport = true
		method, version := jonson.SplitMethodName(object.Name.Name)
		methodName := jonson.GetDefaultMethodName(systemName, method, version)
		fmt.Fprintf(
			wtr,
			`
// %s:%d -- %s
func %s(ctx *jonson.Context%s) %s {
	%s := ctx.CallMethod("%s", %s, %s, nil)
	if err != nil {
		return %s
	}
	%s
	return %s
}
`,
			pos.Filename, pos.Line, object.Name,
			object.Name, params, result,
			vAssign, methodName, rpcHttpMethod, parArg,
			errRet,
			valRet,
			nilRet,
		)
	}

	imports := []string{}
	if needsJonsonImport {
		imports = append(imports, jonsonPath)
	}

	fContent := prependHeader(wtr, pkgName, imports...)

	return writeFile(fNameProcedure, fContent)
}

func firstToLower(s string) string {

	if len(s) == 0 {
		return s
	}

	r := []rune(s)
	r[0] = unicode.ToLower(r[0])

	return string(r)
}

func writeTypesFile(fset *token.FileSet, pkgName string, listTypes []*ast.Object) error {
	needsJonsonImport := false
	wtr := bytes.NewBuffer([]byte{})

	// walk types
	for _, object := range listTypes {
		pos := fset.Position(object.Pos())
		t := object.Decl.(*ast.TypeSpec).Type

		var ts, typeName string
		if _, ok := t.(*ast.InterfaceType); ok {
			ts = "interface" + object.Name
			typeName = object.Name
		} else if _, ok := t.(*ast.StructType); ok {
			ts = "struct"
			typeName = "*" + object.Name
		} else {
			continue
		}

		needsJonsonImport = true
		fmt.Fprintf(
			wtr,
			`
// %s:%d -- %s -- %s
var Type%s = reflect.TypeOf((*%s)(nil)).Elem()

func Require%s(ctx *jonson.Context) %s {
	if v := ctx.Require(Type%s).(%s); v != nil {
		return v
	}
	return nil
}
`,
			pos.Filename, pos.Line,
			object.Name,
			ts,
			object.Name,
			typeName,
			object.Name,
			typeName,
			object.Name,
			typeName,
		)
	}

	imports := []string{}
	if len(listTypes) > 0 {
		imports = append(imports, "reflect")
	}
	if needsJonsonImport {
		imports = append(imports, "github.com/doejon/jonson")
	}

	fContent := prependHeader(wtr, pkgName, imports...)

	return writeFile(fNameProvider, fContent)
}

func main() {
	pkg, fset, err := readPackage()
	if err != nil {
		log.Fatalf("error: %s", err)
	}

	var (
		dc             = doc.New(pkg, fpath, 0)
		whitelistTypes []string
		listTypes      []*ast.Object
		listMethods    []*ast.FuncDecl
		structs        []*ast.Object
	)

	for _, t := range dc.Types {
		if apiTypeMatcher.MatchString(t.Doc) {
			whitelistTypes = append(whitelistTypes, t.Name)
		}
	}

	for _, f := range pkg.Files {
		for _, d := range f.Decls {
			// methods
			if fn, ok := d.(*ast.FuncDecl); ok {
				if apiNameMatcher.MatchString(fn.Name.Name) {
					listMethods = append(listMethods, fn)
				}
			}
		}

		for name, object := range f.Scope.Objects {
			if object.Kind == ast.Typ && ast.IsExported(name) {
				structs = append(structs, object)
			}
			// search types
			if object.Kind == ast.Typ && ast.IsExported(name) && inList(name, whitelistTypes) {
				listTypes = append(listTypes, object)
				continue
			}
		}
	}

	// sort by position asc
	sort.Slice(listTypes, func(i, j int) bool { return listTypes[i].Pos() < listTypes[j].Pos() })
	sort.Slice(listMethods, func(i, j int) bool { return listMethods[i].Pos() < listMethods[j].Pos() })

	if err := writeProcedureFile(fset, pkg.Name, listMethods, structs); err != nil {
		log.Fatalf("error: %s", err)
	}

	if err := writeTypesFile(fset, pkg.Name, listTypes); err != nil {
		log.Fatalf("error: %s", err)
	}

}

func getRpcHttpMethod(decl *ast.FuncDecl) string {
	var m string = "jonson.RpcHttpMethodPost"
	for _, v := range decl.Type.Params.List {
		intf, ok := v.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		switch intf.Sel.Name {
		case "HttpGet":
			return "jonson.RpcHttpMethodGet"
		case "HttpPost":
			return "jonson.RpcHttpMethodPost"
		}

	}

	return m
}
