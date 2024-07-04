package jonson

import (
	"strings"
)

// ToPascalCase converts the provided string to PascalCase
func ToPascalCase(input string) string {
	if input == "" {
		return ""
	}
	kebabCase := ToKebabCase(input)
	words := strings.Split(kebabCase, "-")

	pascal := ""
	for _, word := range words {
		pascal += strings.ToUpper(word[0:1]) + word[1:]
	}
	return pascal
}
