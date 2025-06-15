package plugins

import (
	"bytes"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

type SimpleStruct struct {
	Name string
	Tags []string
}

type ComplexStruct struct {
	ID      int
	Profile *SimpleStruct
	Data    []SimpleStruct
}

type UnexportedFieldStruct struct {
	internalTags []string
	PublicTags   []string
}

type CyclicStructA struct {
	Name    string
	Friends []string
	B       *CyclicStructB
}

type CyclicStructB struct {
	Name    string
	Secrets []string
	A       *CyclicStructA
}

func TestNilSliceNormalizer_Mutate(t *testing.T) {
	testCases := []struct {
		name             string
		plugin           *NilSliceNormalizer
		input            any
		expected         any
		logShouldContain string
	}{
		{
			name:   "Simple struct with nil slice",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input:  &SimpleStruct{Name: "test1", Tags: nil},
			expected: &SimpleStruct{
				Name: "test1",
				Tags: make([]string, 0),
			},
			logShouldContain: `"msg":"Found nil slice in response, normalizing to empty array ` + "`[]`" + `","path":"response.Tags"`,
		},
		{
			name:   "Struct with already empty slice should not change",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input:  &SimpleStruct{Name: "test2", Tags: make([]string, 0)},
			expected: &SimpleStruct{
				Name: "test2",
				Tags: make([]string, 0),
			},
			logShouldContain: "",
		},
		{
			name:   "Struct with populated slice should not change",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input:  &SimpleStruct{Name: "test3", Tags: []string{"a", "b"}},
			expected: &SimpleStruct{
				Name: "test3",
				Tags: []string{"a", "b"},
			},
			logShouldContain: "",
		},
		{
			name:   "Nested struct with nil slice",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input: &ComplexStruct{
				ID:      100,
				Profile: &SimpleStruct{Name: "nested", Tags: nil},
				Data:    nil,
			},
			expected: &ComplexStruct{
				ID: 100,
				Profile: &SimpleStruct{
					Name: "nested",
					Tags: make([]string, 0),
				},
				Data: make([]SimpleStruct, 0),
			},
			// We will get two log entries, so we check for the most specific one.
			logShouldContain: `"path":"response.Profile.Tags"`,
		},
		{
			name:   "Slice of structs with some nil slices",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelInfo},
			input: &ComplexStruct{
				Data: []SimpleStruct{
					{Name: "data1", Tags: []string{"a"}},
					{Name: "data2", Tags: nil},
				},
			},
			expected: &ComplexStruct{
				Data: []SimpleStruct{
					{Name: "data1", Tags: []string{"a"}},
					{Name: "data2", Tags: make([]string, 0)},
				},
			},
			logShouldContain: `"path":"response.Data[].Tags"`,
		},
		{
			name:   "Nil pointer in struct should be ignored but other fields normalized",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input: &ComplexStruct{
				ID:      102,
				Profile: nil,
				Data:    nil,
			},
			expected: &ComplexStruct{
				ID:      102,
				Profile: nil,
				Data:    make([]SimpleStruct, 0),
			},
			logShouldContain: `"path":"response.Data"`,
		},
		{
			name:             "Logging disabled with LogLevelNone",
			plugin:           &NilSliceNormalizer{LogLevel: LogLevelNone},
			input:            &SimpleStruct{Name: "test-no-log", Tags: nil},
			expected:         &SimpleStruct{Name: "test-no-log", Tags: make([]string, 0)},
			logShouldContain: "",
		},
		{
			name:             "Invalid LogLevel should produce a warning about the level",
			plugin:           &NilSliceNormalizer{LogLevel: "invalid-level"},
			input:            &SimpleStruct{Name: "test-invalid-log", Tags: nil},
			expected:         &SimpleStruct{Name: "test-invalid-log", Tags: make([]string, 0)},
			logShouldContain: "Unknown LogLevel in NilSliceNormalizer",
		},
		{
			name:   "Unexported field should be ignored",
			plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input: &UnexportedFieldStruct{
				internalTags: nil,
				PublicTags:   nil,
			},
			expected: &UnexportedFieldStruct{
				internalTags: nil,
				PublicTags:   make([]string, 0),
			},
			logShouldContain: `"path":"response.PublicTags"`,
		},
		{
			name:             "Nil input should not panic",
			plugin:           &NilSliceNormalizer{LogLevel: LogLevelWarn},
			input:            nil,
			expected:         nil,
			logShouldContain: "",
		},
	}

	// Add cyclic test case
	cyclicNodeA := &CyclicStructA{Name: "A", Friends: nil}
	cyclicNodeB := &CyclicStructB{Name: "B", Secrets: nil}
	cyclicNodeA.B = cyclicNodeB
	cyclicNodeB.A = cyclicNodeA
	testCases = append(testCases, struct {
		name             string
		plugin           *NilSliceNormalizer
		input            any
		expected         any
		logShouldContain string
	}{
		name:   "Cyclic struct should not cause infinite loop",
		plugin: &NilSliceNormalizer{LogLevel: LogLevelWarn},
		input:  cyclicNodeA,
		expected: &CyclicStructA{
			Name:    "A",
			Friends: make([]string, 0),
			B: &CyclicStructB{
				Name:    "B",
				Secrets: make([]string, 0),
				A:       cyclicNodeA,
			},
		},
		// We can check for either log message, they will both be present.
		logShouldContain: `"path":"response.Friends"`,
	})

	// Run all test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			// Use a handler that doesn't add the time for more predictable log output.
			logger := slog.New(slog.NewJSONHandler(&logBuffer, &slog.HandlerOptions{
				ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
					if a.Key == slog.TimeKey {
						return slog.Attr{} // Omit the time attribute
					}
					return a
				},
			}))

			tc.plugin.Mutate(tc.input, logger)

			// 1. Assert data equality
			if !reflect.DeepEqual(tc.input, tc.expected) {
				t.Errorf("data mismatch:\ngot:  %#v\nwant: %#v", tc.input, tc.expected)
			}

			// 2. Assert log content
			logOutput := logBuffer.String()
			if tc.logShouldContain != "" && !strings.Contains(logOutput, tc.logShouldContain) {
				t.Errorf("expected log to contain substring:\n--- want ---\n%s\n--- got ---\n%s", tc.logShouldContain, logOutput)
			}
			if tc.logShouldContain == "" && logOutput != "" {
				t.Errorf("expected no log output, but got: %q", logOutput)
			}
		})
	}
}
