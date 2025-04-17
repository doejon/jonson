package jonson

import (
	"fmt"
	"reflect"
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
func Validate(ctx *Context, secret Secret, validateable ValidatedParams, basePath ...string) *Error {
	collector := NewValidator(ctx, secret, basePath...)
	validateable.JonsonValidate(collector)
	return collector.Error()
}
