package jonson

import (
	"fmt"
	"reflect"
	"strings"
)

// ValidatedParams are parameters which have been validated before reaching the business logic.
// Make sure you do keep the validation to static data analysis and do not integrate
// any business logic validation into the validator
type ValidatedParams interface {
	JonsonValidate(validator *Validator)
}

type AllowUnknownFieldsParams interface {
	JonsonAllowUnknownFields()
}

type Validator struct {
	errors   []*Error
	secret   Secret
	basePath []string
	touched  map[string]struct{} // top-level paths referenced via Path()

	*Context
}

// NewValidator returns a new validator;
// Please note: not all features from the context will be available
// within the scope of the validator;
// Ideally, use the context to require logger and time only
func NewValidator(ctx *Context, secret Secret, basePath ...string) *Validator {
	return &Validator{
		errors:   []*Error{},
		secret:   secret,
		basePath: basePath,
		touched:  map[string]struct{}{},
		Context:  ctx,
	}
}

type validatorError struct {
	validator *Validator
	added     bool

	path    []string
	debug   string
	code    int
	message string
}

// add makes sure the error has been added to the
// validator
func (e *validatorError) add() {
	if e.added {
		panic("validator: cannot add error twice")
	}
	e.added = true

	var data *ErrorData

	if len(e.path) > 0 || len(e.debug) > 0 {
		data = &ErrorData{
			Path:  e.path,
			Debug: e.debug,
		}
	}
	err := &Error{
		Code:    e.code,
		Message: e.message,
		Data:    data,
	}
	e.validator.errors = append(e.validator.errors, err)
}

// Message adds an error message which will be forwarded
// to the caller in cleartext
func (e *validatorError) Message(message string) *Validator {
	e.message = message
	e.add()
	return e.validator
}

// Code sets the validator path's code
// which will be set in case the default ErrInvalidParams.Code is not sufficient
func (e *validatorError) Code(c int) *validatorError {
	e.code = c
	return e
}

// Debug adds an encrypted debug string
func (e *validatorError) Debug(s string) *validatorError {
	e.debug = e.validator.secret.Encode(s)
	return e
}

// Validate an validateable item.
// In case the validation returns an error,
// the error will automatically be added to the
// underlying error collector.
// In certain situations you might want to abort
// the validation of proceeding values;
// Therefore, the Validate() function returns the
// validation error in case any occured.
func (e *validatorError) Validate(validateable ValidatedParams) *Error {
	if e.added {
		panic("cannot call validate after adding the error")
	}

	// mark this validator error as done
	e.added = true

	err := Validate(
		e.validator.Context,
		e.validator.secret,
		validateable,
		append(e.validator.basePath, e.path...)...,
	)
	e.added = true
	if err == nil {
		return nil
	}

	// details will always exist since the validator will set the details
	e.validator.errors = append(e.validator.errors, err.Data.Details...)
	return err
}

// Check adds message as error in case ok is false.
// Unlike `if !ok { v.Path(...).Message(...) }`, Check always
// marks the path as validated, which is required for
// MethodHandlerOptions.MissingFieldValidationLevel
func (e *validatorError) Check(ok bool, message string) *Validator {
	if !ok {
		return e.Message(message)
	}
	if e.added {
		panic("validator: cannot add error twice")
	}
	e.added = true
	return e.validator
}

// Skip explicitly marks the path as not requiring validation
func (e *validatorError) Skip() *Validator {
	return e.Check(true, "")
}

type validatorIndex struct {
	index int
}

func (e *Validator) Index(idx int) *validatorIndex {
	return &validatorIndex{
		index: idx,
	}
}

// Path sets the current path that's been validated
func (e *Validator) Path(_path ...any) *validatorError {
	convertedPaths := make([]string, len(_path))
	for i, v := range _path {
		switch x := v.(type) {
		case string:
			convertedPaths[i] = x
		case *validatorIndex:
			convertedPaths[i] = fmt.Sprintf("[%d]", x.index)
		default:
			panic(fmt.Sprintf("unsupported path type: %v; string, validator.Index() are the only supported types", reflect.TypeOf(v)))
		}
	}

	if len(convertedPaths) > 0 {
		e.touched[convertedPaths[0]] = struct{}{}
	}

	return &validatorError{
		path:      append(e.basePath, convertedPaths...),
		validator: e,

		code:    ErrInvalidParams.Code,
		message: ErrInvalidParams.Message,
	}
}

// Error returns a single error which combines all
// the errors that have been collected.
// In case no error has been collected, Error returns nil
func (e *Validator) Error() *Error {
	if len(e.errors) <= 0 {
		return nil
	}
	return ErrInvalidParams.CloneWithData(&ErrorData{
		Details: e.errors,
	})
}

// Validate validates the handled interface
func Validate(
	ctx *Context,
	secret Secret,
	validateable ValidatedParams,
	basePath ...string,
) *Error {
	collector := NewValidator(ctx, secret, basePath...)
	validateable.JonsonValidate(collector)
	return collector.Error()
}

// jsonFieldNames returns the json names of all exported fields of struct t,
// flattening embedded structs without json tag (e.g. jonson.Params)
func jsonFieldNames(t reflect.Type) []string {
	out := []string{}
	for f := range t.Fields() {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			ft := f.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				out = append(out, jsonFieldNames(ft)...)
				continue
			}
		}
		if !f.IsExported() {
			continue
		}
		if name == "" {
			name = f.Name
		}
		out = append(out, name)
	}
	return out
}

// unvalidatedFields runs JonsonValidate on a zero value of t (struct type)
// and returns all json fields which have not been referenced via Path().
// Only unconditional calls (Check, Skip, Validate) are guaranteed to be seen.
func unvalidatedFields(ctx *Context, secret Secret, t reflect.Type) (missing []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("JonsonValidate panicked on zero value: %v", r)
		}
	}()
	v := NewValidator(ctx, secret)
	reflect.New(t).Interface().(ValidatedParams).JonsonValidate(v)
	for _, name := range jsonFieldNames(t) {
		if _, ok := v.touched[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing, nil
}
