package jonson

import (
	"regexp"
	"strings"
)

var (
	matchFirstCap = regexp.MustCompile("([A-Z])([A-Z][a-z])")
	matchAllCap   = regexp.MustCompile("([a-z0-9])([A-Z])")
)

// ToKebabCase converts the provided string to kebab-case
func ToKebabCase(input string) string {
	output := matchFirstCap.ReplaceAllString(input, "${1}-${2}")
	output = matchAllCap.ReplaceAllString(output, "${1}-${2}")
	output = strings.ReplaceAll(output, "_", "-")
	return strings.ToLower(output)
}
