package plugins

import (
	"github.com/doejon/jonson"
	"log/slog"
	"reflect"
)

// LogLevel defines the logging verbosity for the normalizer.
type LogLevel string

const (
	LogLevelNone  LogLevel = "none"
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
)

// Compile-time check to ensure *NilSliceNormalizer implements the jonson.ResponseMutator interface.
var _ jonson.ResponseMutator = (*NilSliceNormalizer)(nil)

type NilSliceNormalizer struct {
	LogLevel LogLevel
}

func (n *NilSliceNormalizer) Mutate(result any, logger *slog.Logger) {
	visited := make(map[uintptr]bool)
	n.normalizeRecursive(reflect.ValueOf(result), visited, logger, "response")
}

// normalizeRecursive recursively traverses the value and normalizes nil slices to empty slices.
func (n *NilSliceNormalizer) normalizeRecursive(val reflect.Value, visited map[uintptr]bool, logger *slog.Logger, path string) {
	// If value is invalid (e.g. from a nil interface), stop.
	if !val.IsValid() {
		return
	}

	// First, handle pointers to traverse to the actual data.
	if val.Kind() == reflect.Ptr {
		if val.IsNil() {
			return
		}
		ptrAddr := val.Pointer()
		if visited[ptrAddr] {
			return // Cycle detected.
		}
		visited[ptrAddr] = true
		n.normalizeRecursive(val.Elem(), visited, logger, path)
		return
	}

	// Now, switch on the concrete kind of the value.
	switch val.Kind() {
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			// Recurse into each field of the struct.
			newPath := path + "." + val.Type().Field(i).Name
			n.normalizeRecursive(val.Field(i), visited, logger, newPath)
		}

	case reflect.Slice:
		if val.IsNil() {
			if val.CanSet() {
				// Log with the configured level.
				if n.LogLevel != LogLevelNone {
					msg := "Found nil slice in response, normalizing to empty array `[]`"
					attrs := slog.String("path", path)
					switch n.LogLevel {
					case LogLevelDebug:
						logger.Debug(msg, attrs)
					case LogLevelInfo:
						logger.Info(msg, attrs)
					case LogLevelWarn:
						logger.Warn(msg, attrs)
					default:
						logger.Warn("Unknown LogLevel in NilSliceNormalizer, defaulting to Warn for notification", "configuredLevel", n.LogLevel)
						logger.Warn(msg, attrs)
					}
				}
				// Replace the nil slice with an empty one.
				val.Set(reflect.MakeSlice(val.Type(), 0, 0))
			}
			return // Normalization done, no elements to inspect.
		}
		// If the slice is not nil, recurse into its elements.
		for i := 0; i < val.Len(); i++ {
			n.normalizeRecursive(val.Index(i), visited, logger, path+"[]")
		}

	case reflect.Map:
		if val.IsNil() {
			return
		}
		// Recurse into each value of the map. The user requested not to normalize
		// nil maps to {}, but we still need to inspect their contents.
		for _, key := range val.MapKeys() {
			n.normalizeRecursive(val.MapIndex(key), visited, logger, path+"{}")
		}

	case reflect.Interface:
		if val.IsNil() {
			return
		}
		// Look inside the interface to find the concrete value and recurse.
		n.normalizeRecursive(val.Elem(), visited, logger, path)

	default:
		// No action is needed, simply stop the recursion for this branch.
		return
	}
}
