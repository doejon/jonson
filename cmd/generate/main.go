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
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go/format"

	"github.com/doejon/jonson"
)

var (
	fpath          = "."
	apiNameMatcher = regexp.MustCompile(`^(.+)V([0-9]+)$`)
	apiTypeMatcher = regexp.MustCompile(`(^|\n)@generate($|\n)`)
)

func init() {
	flag.StringVar(&fpath, "path", fpath, "filepath to scan")
	flag.Parse()
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "Error: "+err.Error())
	os.Exit(-1)
}

func inList(s string, list []string) bool {
	for _, cmp := range list {
		if s == cmp {
			return true
		}
	}
	return false
}

func main() {
	stat, err := os.Stat(fpath)
	if err != nil {
		exitErr(err)
	}
	if !stat.IsDir() {
		exitErr(errors.New("valid path file expected"))
	}

	// parse ast
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, fpath, func(fi os.FileInfo) bool {
		return fi.Name() != "def.gen.go"
	}, parser.ParseComments)
	if err != nil {
		exitErr(err)
	}

	// search for type declarations
	if len(pkgs) != 1 {
		exitErr(errors.New("exactly 1 package expected"))
	}
	var pkg *ast.Package
	for _, p := range pkgs {
		pkg = p
	}

	var (
		dc             = doc.New(pkg, fpath, 0)
		whitelistTypes []string
		listTypes      []*ast.Object
		listMethods    []*ast.FuncDecl
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
			// search types
			if object.Kind == ast.Typ && ast.IsExported(name) && inList(name, whitelistTypes) {
				//t := object.Decl.(*ast.TypeSpec)
				//fmt.Printf("***\nNam: %s\nDoc: %#v\nCom: %#v\n", t.Name.Name, t.Doc, t.Comment)
				listTypes = append(listTypes, object)
				continue
			}
		}
	}

	// sort by position asc
	sort.Slice(listTypes, func(i, j int) bool { return listTypes[i].Pos() < listTypes[j].Pos() })
	sort.Slice(listMethods, func(i, j int) bool { return listMethods[i].Pos() < listMethods[j].Pos() })

	// open output file
	outputFilename := filepath.Join(fpath, "def.gen.go")
	fp, err := os.OpenFile(outputFilename, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		exitErr(err)
	}

	wtr := bytes.NewBuffer([]byte{})

	defer func() {
		b := wtr.Bytes()
		var err error
		b, err = format.Source(b)
		if err != nil {
			panic("failed to format source file: " + err.Error())
		}

		if _, err := fp.Write(b); err != nil {
			panic("failed to write to file: " + err.Error())
		}
		if err := fp.Close(); err != nil {
			panic("failed to store file: " + err.Error())
		}
	}()

	needsJonsonImport := false
	// walk methods
	for _, object := range listMethods {
		pos := fset.Position(object.Pos())

		systemName := jonson.ToKebabCase(object.Recv.List[0].Type.(*ast.StarExpr).X.(*ast.Ident).Name)

		// input
		var paramType string
		if len(object.Type.Params.List) > 0 {
			lastArg := object.Type.Params.List[len(object.Type.Params.List)-1]
			if starExpr, ok := lastArg.Type.(*ast.StarExpr); ok {
				if ident, ok := starExpr.X.(*ast.Ident); ok && ident.Obj != nil {
					if decl, ok := ident.Obj.Decl.(*ast.TypeSpec); ok {
						if stru, ok := decl.Type.(*ast.StructType); ok {
							if stru.Fields == nil || len(stru.Fields.List) <= 0 {
								panic(fmt.Errorf("missing jonson.Params field in struct %s", decl.Name.String()))
							} else if p, ok := stru.Fields.List[0].Type.(*ast.SelectorExpr); ok {
								if p.X.(*ast.Ident).Name == "jonson" && p.Sel.Name == "Params" {
									paramType = ident.Name
								}
							} else {
								panic(fmt.Errorf("jonson.Params field in struct %s, should be first field", decl.Name.String()))
							}
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
	%s := ctx.CallMethod("%s", %s, nil)
	if err != nil {
		return %s
	}
	%s
	return %s
}
`,
			pos.Filename, pos.Line, object.Name,
			object.Name, params, result,
			vAssign, methodName, parArg,
			errRet,
			valRet,
			nilRet,
		)
	}

	fmt.Fprintf(
		wtr,
		`
// ---- type handling wrappers ----
`,
	)

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

	// once done, let's prepend the file header

	// file header
	imports := []string{}
	if len(listTypes) > 0 {
		imports = append(imports, "\"reflect\"")
	}
	if needsJonsonImport {
		imports = append(imports, "\"github.com/doejon/jonson\"")
	}

	fileContent := wtr.String()
	wtr.Reset()

	fmt.Fprint(
		wtr,
		`// code generated by jonson-generate; DO NOT EDIT.
		
package `+pkg.Name+`

import (
	`+strings.Join(imports, "\n")+`
)

// ---- method call wrappers ----
`)
	wtr.WriteString(fileContent)
}
