package jonson

import "testing"

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"camelCase", "CamelCase"},
		{"PascalCase", "PascalCase"},
		{"snake_case", "SnakeCase"},
		{"Pascal_Snake", "PascalSnake"},
		{"SCREAMING-SNAKE", "ScreamingSnake"},
		{"kebab-case", "KebabCase"},
		{"Pascal-Kebab", "PascalKebab"},
		{"SCREAMING-KEBAB", "ScreamingKebab"},
		{"A", "A"},
		{"AA", "Aa"},
		{"AAA", "Aaa"},
		{"AAAA", "Aaaa"},
		{"AaAa", "AaAa"},
		{"HTTPRequest", "HttpRequest"},
		{"BatteryLifeValue", "BatteryLifeValue"},
		{"Id0Value", "Id0Value"},
		{"ID0Value", "Id0Value"},
	}
	for _, tt := range tests {
		if got := ToPascalCase(tt.input); got != tt.expected {
			t.Errorf("ToPascalCase() = %v, expected %v", got, tt.expected)
		}
	}
}
