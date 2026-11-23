package jonson

import (
	"encoding/json"
	"reflect"
	"strings"
	"time"
)

var (
	typeTime        = reflect.TypeFor[time.Time]()
	typeRawMessage  = reflect.TypeFor[json.RawMessage]()
	typeJsonNumber  = reflect.TypeFor[json.Number]()
	typeAllowFields = reflect.TypeFor[AllowUnknownFieldsParams]()
)

// mcpInputSchema returns the JSON Schema of a method's params type; nil means no params.
// Field names, embedding and optional fields follow encoding/json: a field without
// omitempty or omitzero is required. The struct tag `description:"..."` documents a field.
// Objects reject unknown fields like the default JsonHandler does, unless the params
// implement AllowUnknownFieldsParams.
// ponytail: no $ref, enums or formats except date-time; add them when a tool needs them.
func mcpInputSchema(params reflect.Type) map[string]any {
	if params == nil {
		return map[string]any{"type": "object"}
	}
	strict := !reflect.PointerTo(params).Implements(typeAllowFields) && !params.Implements(typeAllowFields)
	return jsonSchemaOf(params, strict, map[reflect.Type]bool{})
}

func jsonSchemaOf(t reflect.Type, strict bool, seen map[reflect.Type]bool) map[string]any {
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
		if t.Elem().Kind() == reflect.Uint8 {
			// encoding/json encodes []byte as base64
			return map[string]any{"type": "string"}
		}
		return map[string]any{"type": "array", "items": jsonSchemaOf(t.Elem(), strict, seen)}
	case reflect.Map:
		return map[string]any{"type": "object", "additionalProperties": jsonSchemaOf(t.Elem(), strict, seen)}
	case reflect.Struct:
		if seen[t] {
			// recursive type: stop here instead of looping
			return map[string]any{"type": "object"}
		}
		seen[t] = true
		defer delete(seen, t)
		props := map[string]any{}
		required := []string{}
		addStructFields(t, strict, seen, props, &required)
		out := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			out["required"] = required
		}
		if strict {
			out["additionalProperties"] = false
		}
		return out
	default:
		// interfaces and anything else accept any JSON value
		return map[string]any{}
	}
}

func addStructFields(t reflect.Type, strict bool, seen map[reflect.Type]bool, props map[string]any, required *[]string) {
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
				addStructFields(ft, strict, seen, props, required)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}

		schema := jsonSchemaOf(f.Type, strict, seen)
		if tagHasOption(opts, "string") {
			schema = map[string]any{"type": "string"}
		}
		if d := f.Tag.Get("description"); d != "" {
			schema["description"] = d
		}
		props[name] = schema
		if !tagHasOption(opts, "omitempty") && !tagHasOption(opts, "omitzero") {
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
