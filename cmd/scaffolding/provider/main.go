package main

import (
	"embed"
	"flag"
	"fmt"
	"github.com/doejon/jonson"
	"log"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed *.tmpl
var tfs embed.FS

func main() {

	var providerName string
	var providedType string
	flag.StringVar(&providerName, "providerName", "test", "desired provider name")
	flag.StringVar(&providedType, "providedType", "MyValue", "desired provided type")

	flag.Parse()

	if providerName == "" || providedType == "" {
		flag.PrintDefaults()
		os.Exit(1)
	}

	tmpl, err := template.ParseFS(tfs, "provider.go.tmpl")
	if err != nil {
		log.Fatalf("parsing template: %s", err)
	}

	dstPath := filepath.Join(dirname(), fmt.Sprintf("%s-provider.go", providerName))
	if err := ensureNotOverwriting(dstPath); err != nil {
		log.Fatalf("file %s already exists: %s", dstPath, err)
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		log.Fatalf("create file %s: %w", dstPath, err)
	}
	defer dst.Close()

	data := struct {
		Name string
		Type string
	}{
		Name: jonson.ToPascalCase(providerName),
		Type: jonson.ToPascalCase(providedType),
	}

	if err := tmpl.ExecuteTemplate(dst, tmpl.Name(), data); err != nil {
		log.Fatalf("executing template for %s: %s", dst.Name(), err)
	}
}

func ensureNotOverwriting(filename string) error {
	_, err := os.Stat(filename)
	if err == nil {
		return fmt.Errorf("file already exists: %s", filename)
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("stat file: %s, %w", filename, err)
	}
	return nil
}

func dirname() string {
	getwd, err := os.Getwd()
	if err != nil {
		log.Fatal("getting current directory")
	}

	return getwd
}
