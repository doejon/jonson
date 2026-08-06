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

	var pascal strings.Builder
	for _, word := range words {
		pascal.WriteString(strings.ToUpper(word[0:1]) + word[1:])
	}
	return pascal.String()
}
