package main

import (
	"bytes"
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
