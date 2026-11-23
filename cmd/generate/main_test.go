package main

import (
	"bytes"
	"go/parser"
	"go/token"
	"regexp"
	"strings"
	"testing"
)

func TestGeneratedHeader(t *testing.T) {
	output := prependHeader(bytes.NewBuffer(nil), "example")
	firstLine := strings.SplitN(string(output), "\n", 2)[0]
	if !regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`).MatchString(firstLine) {
		t.Fatalf("generated header %q does not match Go's generated-file convention", firstLine)
	}
}

func TestFieldDescriptions(t *testing.T) {
	src := `package x

type Params struct {
	// Uuid of the
	// account.
	Uuid string
	Age  int // age in "years"
	A, B bool // shared
	hidden int // unexported
	NoComment string
}

type Empty struct{ A int }

type Gen[T any] struct {
	A T // generic types are skipped
}
`
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", src, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	wtr := bytes.NewBuffer(nil)
	writeFieldDescriptions(wtr, f)
	want := `
func (*Params) JonsonFieldDescriptions() map[string]string {
	return map[string]string{
		"Uuid": "Uuid of the account.",
		"Age": "age in \"years\"",
		"A": "shared",
		"B": "shared",
	}
}
`
	if got := wtr.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestGeneratedHeaderWithoutImports(t *testing.T) {
	output := string(prependHeader(bytes.NewBuffer(nil), "example"))
	if strings.Contains(output, "import") {
		t.Fatalf("header without imports contains an import block:\n%s", output)
	}
}
