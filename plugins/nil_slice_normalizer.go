package plugins

import (
	"log/slog"
	"reflect"
	"strings"
)

// LogLevel defines the logging verbosity for the normalizer.
type LogLevel int

const (
	LogLevelNone LogLevel = iota
	LogLevelDebug
	LogLevelInfo
	LogLevelWarn
)

type NilSliceNormalizer struct {
	logLevel LogLevel
	logger   *slog.Logger
}

// compile time check to make sure we're implementing the encode mutator
var _ JsonEncodeMutator = (&NilSliceNormalizer{})

func NewNilSliceNormalizer() *NilSliceNormalizer {
	return &NilSliceNormalizer{
		logLevel: LogLevelNone,
	}
}

func (n *NilSliceNormalizer) WithLogger(logger *slog.Logger) *NilSliceNormalizer {
	n.logger = logger
	return n
}

func (n *NilSliceNormalizer) WithLogLevel(lvl LogLevel) *NilSliceNormalizer {
	n.logLevel = lvl
	return n
}

func (n *NilSliceNormalizer) MutateEncode(data any) {
	visited := make(map[uintptr]bool)
	n.normalizeRecursive(reflect.ValueOf(data), visited, "<root>", false)
}

// normalizeRecursive recursively traverses the value and normalizes nil slices to empty slices.
func (n *NilSliceNormalizer) normalizeRecursive(val reflect.Value, visited map[uintptr]bool, path string, omitempty bool) {
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
			return // cycle detected.
		}
		visited[ptrAddr] = true
		n.normalizeRecursive(val.Elem(), visited, path, omitempty)
		return
	}

	// Now, switch on the concrete kind of the value.
	switch val.Kind() {
	case reflect.Struct:
		for i := 0; i < val.NumField(); i++ {
			// Recurse into each field of the struct.
			tp := val.Type()
			fld := tp.Field(i)
			omitempty := strings.HasSuffix(fld.Tag.Get("json"), ",omitempty")
			newPath := path + "." + fld.Name
			n.normalizeRecursive(val.Field(i), visited, newPath, omitempty)
		}

	case reflect.Slice:
		if omitempty {
			if val.IsNil() {
				return // done
			}
			// omitempty is set, make sure
			// to remove an empty slice
			ln := val.Len()
			canSet := val.CanSet()
			if canSet && ln <= 0 {
				val.SetZero()
				return // done
			}
		} else {
			// do not omitempty, make sure to have a slice at hand
			if val.IsNil() {
				if val.CanSet() {
					// Log with the configured level.
					if n.logger != nil && n.logLevel != LogLevelNone {
						msg := "Found nil slice in response, normalizing to empty array `[]`"
						attrs := slog.String("path", path)
						switch n.logLevel {
						case LogLevelDebug:
							n.logger.Debug(msg, attrs)
						case LogLevelInfo:
							n.logger.Info(msg, attrs)
						case LogLevelWarn:
							n.logger.Warn(msg, attrs)
						default:
							n.logger.Warn("Unknown LogLevel in NilSliceNormalizer, defaulting to Warn for notification", "configuredLevel", n.logLevel)
							n.logger.Warn(msg, attrs)
						}
					}
					// Replace the nil slice with an empty one.
					val.Set(reflect.MakeSlice(val.Type(), 0, 0))
				}
				return // Normalization done, no elements to inspect.
			}
		}

		// If the slice is not nil, recurse into its elements.
		for i := 0; i < val.Len(); i++ {
			n.normalizeRecursive(val.Index(i), visited, path+"[]", false)
		}

	case reflect.Map:
		if val.IsNil() {
			return
		}
		// Recurse into each value of the map. The user requested not to normalize
		// nil maps to {}, but we still need to inspect their contents.
		for _, key := range val.MapKeys() {
			n.normalizeRecursive(val.MapIndex(key), visited, path+"{}", false)
		}

	case reflect.Interface:
		if val.IsNil() {
			return
		}
		// Look inside the interface to find the concrete value and recurse.
		n.normalizeInterface(val, val.Elem(), visited, path, omitempty)

	default:
		// No action is needed, simply stop the recursion for this branch.
		return
	}
}

func (n *NilSliceNormalizer) normalizeInterface(itf reflect.Value, elem reflect.Value, visited map[uintptr]bool, path string, omitempty bool) {
	if elem.IsNil() {
		return
	}

	// dereference all pointers in the interface before continuing
	// with recursive normalization - we might have to set
	// the interface to nil in case we do have pointers to slices
	knd := elem.Kind()
	for {
		if knd != reflect.Pointer {
			break
		}
		ptrAddr := elem.Pointer()
		if visited[ptrAddr] {
			return // cycle detected
		}
		elem = elem.Elem()
		knd = elem.Kind()
	}

	if omitempty && knd == reflect.Slice && elem.Len() <= 0 && itf.CanSet() {
		itf.SetZero()
		return // done
	}

	// our parent is the interface - all ptrs have been dereferenced before continuing
	// with the recursive normalization
	n.normalizeRecursive(elem, visited, path, omitempty)
}
