package jonson

import (
	"testing"
)

func Test_ToKebabCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"camelCase", "camel-case"},
		{"PascalCase", "pascal-case"},
		{"snake_case", "snake-case"},
		{"Pascal_Snake", "pascal-snake"},
		{"SCREAMING-SNAKE", "screaming-snake"},
		{"kebab-case", "kebab-case"},
		{"Pascal-Kebab", "pascal-kebab"},
		{"SCREAMING-KEBAB", "screaming-kebab"},
		{"A", "a"},
		{"AA", "aa"},
		{"AAA", "aaa"},
		{"AAAA", "aaaa"},
		{"AaAa", "aa-aa"},
		{"HTTPRequest", "http-request"},
		{"BatteryLifeValue", "battery-life-value"},
		{"Id0Value", "id0-value"},
		{"ID0Value", "id0-value"},
	}

	for _, test := range tests {
		result := ToKebabCase(test.input)
		if result != test.expected {
			t.Fatalf("expected %s, got %s", test.expected, result)
		}
	}
}
