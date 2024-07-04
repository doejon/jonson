package jonson

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// Validate validates the given object
func Validate(errEncoder Secret, obj any) error {
	if errs := validate(errEncoder, obj, nil); len(errs) > 0 {
		return ErrInvalidParams.CloneWithData(&ErrorData{
			Details: errs,
		})
	}

	return nil
}

func validate(errEncoder Secret, obj any, currentPath []any) []*Error {
	var errs []*Error

	rv := reflect.ValueOf(obj)
	rt := toPointer(reflect.TypeOf(obj))
	rte := rt.Elem()

	for i := 0; i < rte.NumField(); i++ {
		rtf := rte.Field(i)
		if rtf.PkgPath != "" {
			// skip private fields
			continue
		}

		name := rtf.Name

		// extract json field name
		if jt, ok := rtf.Tag.Lookup("json"); ok {
			if p := strings.Split(jt, ","); len(p) > 0 && len(p[0]) > 0 {
				if p[0] == "-" {
					// skip fields we don't want in json
					continue
				}
				name = p[0]
			}
		}

		// build path for current field member
		path := append(append([]any(nil), currentPath...), name)

		// do we have validation happening on the struct's field itself?
		rtv := rv.Field(i)
		if rtv.Kind() == reflect.Struct {
			if nestedStructErrors := validate(errEncoder, rtv.Interface(), path); nestedStructErrors != nil {
				errs = append(errs, nestedStructErrors...)
			}
		}

		// check if valider exists for field
		if m, ok := rt.MethodByName("Validate" + rtf.Name); ok {
			res := m.Func.Call([]reflect.Value{rv})

			if err, ok := res[0].Interface().(*Error); ok && err != nil {
				errs = append(errs, err.CloneWithData(&ErrorData{
					Path: path,
				}))
				continue
			}

			if err, ok := res[0].Interface().(error); ok && err != nil {
				errs = append(errs, ErrInternal.CloneWithData(&ErrorData{
					Path:  path,
					Debug: errEncoder.Encode(err.Error()),
				}))
				continue
			}
		}
	}

	return errs
}

func toPointer(rt reflect.Type) reflect.Type {
	if rt.Kind() == reflect.Pointer {
		return rt
	}
	return reflect.PointerTo(rt)
}

// TODO
func implementsValidator(rt reflect.Type, currentPath []any) []error {
	// in case the past element hasn't been a pointer,
	// we need to make a pointer out of the value;
	// Functions attached to a structh which is not accessible with a pointer
	// will otherwise stay ignored
	rt = toPointer(rt)
	rte := rt.Elem()

	if rte.Kind() != reflect.Struct {
		return []error{errors.New("validator needs to be of type struct, got: " + rt.String())}
	}

	var errs []error

	for i := 0; i < rte.NumField(); i++ {
		rtField := rte.Field(i)
		if rtField.PkgPath != "" {
			// skip private fields
			continue
		}

		name := rtField.Name

		// extract json field name
		if jt, ok := rtField.Tag.Lookup("json"); ok {
			if p := strings.Split(jt, ","); len(p) > 0 && len(p[0]) > 0 {
				if p[0] == "-" {
					// skip fields we don't want in json
					continue
				}
				name = p[0]
			}
		}

		// build path for current field member
		path := append(append([]any(nil), currentPath...), name)

		if _, ok := rt.MethodByName("Validate" + rtField.Name); !ok {
			// nope, no explicit validator set for the key itself.
			// in case the key is a struct itself, does it implement a validator?
			fieldType := rtField.Type
			if fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}

			if fieldType.Kind() == reflect.Struct {
				if nestedStructErrors := implementsValidator(rtField.Type, path); nestedStructErrors != nil {
					errs = append(errs, nestedStructErrors...)
				}
			} else if fieldType.Kind() == reflect.Slice {
				// TODO: validate slice

			} else {
				errs = append(errs, fmt.Errorf("missing validation method: Validate%s in path %v", rtField.Name, path))
			}
		}
	}
	if len(errs) <= 0 {
		return nil
	}

	return errs
}
