package jonson

import (
	"encoding"
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

var (
	typeTime              = reflect.TypeFor[time.Time]()
	typeRawMessage        = reflect.TypeFor[json.RawMessage]()
	typeJsonNumber        = reflect.TypeFor[json.Number]()
	typeAllowFields       = reflect.TypeFor[AllowUnknownFieldsParams]()
	typeFieldDescriptions = reflect.TypeFor[FieldDescriber]()
	typeJsonMarshaler     = reflect.TypeFor[json.Marshaler]()
	typeJsonUnmarshaler   = reflect.TypeFor[json.Unmarshaler]()
	typeTextMarshaler     = reflect.TypeFor[encoding.TextMarshaler]()
	typeTextUnmarshaler   = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// implements reports whether t or *t implements iface.
func implements(t reflect.Type, iface reflect.Type) bool {
	return t.Implements(iface) || reflect.PointerTo(t).Implements(iface)
}

// mcpInputSchema returns the JSON Schema of a method's params type; nil means no params.
// Field names, embedding and optional fields follow encoding/json: a field without
// omitempty or omitzero is required. The struct tag `description:"..."` documents a field,
// falling back to FieldDescriber (generated from field comments).
// Objects reject unknown fields like the default JsonHandler does, unless the params
// implement AllowUnknownFieldsParams.
// ponytail: no $ref, enums or formats except date-time; add them when a tool needs them.
func mcpInputSchema(params reflect.Type) map[string]any {
	if params == nil {
		return map[string]any{"type": "object"}
	}
	strict := !reflect.PointerTo(params).Implements(typeAllowFields) && !params.Implements(typeAllowFields)
	schema := jsonSchemaOf(params, schemaMode{strict: strict}, map[reflect.Type]bool{})
	// tool arguments are always an object, even if the params decode themselves
	if schema["type"] != "object" {
		return map[string]any{"type": "object"}
	}
	return schema
}

// mcpOutputSchema returns the JSON Schema of a method's result type; nil means no result.
// It describes what encoding/json writes: a field without omitempty or omitzero is always
// present, and nil pointers, slices and maps are written as null.
// ponytail: nil elements inside slices and maps ([]*T) are not marked nullable; add it when a tool returns them.
func mcpOutputSchema(result reflect.Type) map[string]any {
	if result == nil {
		return nil
	}
	schema := jsonSchemaOf(result, schemaMode{output: true}, map[reflect.Type]bool{})
	// a result that encodes itself has no known shape
	if schema["type"] != "object" {
		return nil
	}
	return schema
}

// schemaMode tells jsonSchemaOf whether it describes params (input) or a result (output).
type schemaMode struct {
	// strict rejects unknown object fields
	strict bool
	// output describes encoded values instead of accepted ones
	output bool
}

func jsonSchemaOf(t reflect.Type, mode schemaMode, seen map[reflect.Type]bool) map[string]any {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t {
	case typeTime:
		return map[string]any{"type": "string", "format": "date-time"}
	case typeRawMessage:
		return map[string]any{}
	case typeJsonNumber:
		return map[string]any{"type": "number"}
	}
	// custom encodings decide their own shape, which reflection can't see;
	// encoding/json only uses the methods for its direction
	jsonCodec, textCodec := typeJsonUnmarshaler, typeTextUnmarshaler
	if mode.output {
		jsonCodec, textCodec = typeJsonMarshaler, typeTextMarshaler
	}
	if implements(t, jsonCodec) {
		return map[string]any{}
	}
	if implements(t, textCodec) {
		return map[string]any{"type": "string"}
	}
	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice, reflect.Array:
		if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
			// encoding/json encodes []byte as base64, but [N]byte as an array of numbers
			return map[string]any{"type": "string"}
		}
		return map[string]any{"type": "array", "items": jsonSchemaOf(t.Elem(), mode, seen)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": jsonSchemaOf(t.Elem(), mode, seen)}
	case reflect.Struct:
		if seen[t] {
			// recursive type: stop here instead of looping
			return map[string]any{"type": "object"}
		}
		seen[t] = true
		defer delete(seen, t)
		props := map[string]any{}
		required := []string{}
		addStructFields(t, mode, false, seen, props, &required)
		out := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			out["required"] = required
		}
		if mode.strict {
			out["additionalProperties"] = false
		}
		return out
	default:
		// interfaces and anything else accept any JSON value
		return map[string]any{}
	}
}

// addStructFields adds the fields of t to props. Fields of an embedded pointer are optional:
// encoding/json leaves them out when the pointer is nil, and doesn't need them to decode.
func addStructFields(t reflect.Type, mode schemaMode, optional bool, seen map[reflect.Type]bool, props map[string]any, required *[]string) {
	var descriptions map[string]string
	if reflect.PointerTo(t).Implements(typeFieldDescriptions) {
		descriptions = reflect.New(t).Interface().(FieldDescriber).JonsonFieldDescriptions()
	}
	for i := range t.NumField() {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name, opts, _ := strings.Cut(tag, ",")

		// embedded structs without a json name are inlined, like encoding/json does
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				addStructFields(ft, mode, optional || f.Type.Kind() == reflect.Pointer, seen, props, required)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}

		schema := jsonSchemaOf(f.Type, mode, seen)
		if tagHasOption(opts, "string") {
			schema = map[string]any{"type": "string"}
		}
		if d := f.Tag.Get("description"); d != "" {
			schema["description"] = d
		} else if d := descriptions[f.Name]; d != "" {
			schema["description"] = d
		}
		props[name] = schema
		omittable := tagHasOption(opts, "omitempty") || tagHasOption(opts, "omitzero")
		nilable := f.Type.Kind() == reflect.Pointer || f.Type.Kind() == reflect.Slice || f.Type.Kind() == reflect.Map
		switch {
		case omittable:
		case mode.output:
			// always written, as null when nil
			if typ, ok := schema["type"].(string); ok && nilable {
				schema["type"] = []string{typ, "null"}
			}
			if !optional {
				*required = append(*required, name)
			}
		case !nilable && !optional:
			// a missing pointer, slice or map decodes to nil, so the server accepts it
			*required = append(*required, name)
		}
	}
}

// tagHasOption reports whether the comma separated json tag options contain opt.
func tagHasOption(opts string, opt string) bool {
	for o := range strings.SplitSeq(opts, ",") {
		if o == opt {
			return true
		}
	}
	return false
}
