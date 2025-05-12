package jonson

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Error object
type Error struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *ErrorData `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return e.Message + " (" + strconv.FormatInt(int64(e.Code), 10) + ")"
}

// CloneWithData returns a copy with the supplied data set
func (e Error) CloneWithData(data *ErrorData) *Error {
	e.Data = data
	return &e
}

// String implements stringer
func (e Error) String() string {
	data := ""
	if e.Data != nil {
		data = e.Data.String()
	}
	return fmt.Sprintf("error code: %d | message: %s | data: %s", e.Code, e.Message, data)
}

func (e *Error) Inspect() *ErrorInspector {
	return NewErrorInspector(e)
}

// indents a block of text with an indent string
func indent(text, indent string) string {
	if text[len(text)-1:] == "\n" {
		result := ""
		for _, j := range strings.Split(text[:len(text)-1], "\n") {
			result += indent + j + "\n"
		}
		return result
	}
	result := ""
	for _, j := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		result += indent + j + "\n"
	}
	return result[:len(result)-1]
}

// ErrorData object
type ErrorData struct {
	Path    []string `json:"path,omitempty"`
	Details []*Error `json:"details,omitempty"`
	Debug   string   `json:"debug,omitempty"`
}

func (e ErrorData) String() string {
	paths := []string{}
	for _, v := range e.Path {
		paths = append(paths, fmt.Sprintf("%v", v))
	}
	details := []string{}
	for _, v := range e.Details {
		details = append(details, indent(v.String(), "  "))
	}
	if len(paths) <= 0 && len(details) <= 0 && len(e.Debug) <= 0 {
		return ""
	}

	return fmt.Sprintf("\n---\npath: %s\n---\ndetails:\n---\n%s\n---\ndebug: %s", strings.Join(paths, "\n"), strings.Join(details, "\n"), e.Debug)
}

// PanicError is an error created in case the executing rpc message panics
type PanicError struct {
	Err    error
	Stack  string
	ID     json.RawMessage
	Method string
}

func (p *PanicError) Error() string {
	return p.Err.Error()
}

// ErrorInspector allows for inspecting an Error object by searching for specific errors
type ErrorInspector struct {
	rootErr *Error

	// matchers
	code    *int
	path    *[]string
	message *string
}

// NewErrorInspector returns a new error inspector
func NewErrorInspector(root *Error) *ErrorInspector {
	return &ErrorInspector{
		rootErr: root,
	}
}

// Code compares an error's code
func (e *ErrorInspector) Code(code int) *ErrorInspector {
	e.code = &code
	return e
}

// Path compares an error's path
func (e *ErrorInspector) Path(path ...string) *ErrorInspector {
	sl := append([]string{}, path...)
	e.path = &sl
	return e
}

// Message compares an error's message
func (e *ErrorInspector) Message(message string) *ErrorInspector {
	e.message = &message
	return e
}

// Error matches the error code only and ignores all other
// parts of the provided err. Instead of using matches.Code(err.Code),
// you can use matches.Error(err)
func (e *ErrorInspector) Error(err *Error) *ErrorInspector {
	cpy := err.Code
	e.code = &cpy
	return e
}

// FindFirst returns first result in error that fulfills given errors.
// In case no error could be found, nil, error will be returned
func (e *ErrorInspector) FindFirst() (*Error, error) {
	out := e.findFirst(e.rootErr)
	if out == nil {
		return nil, fmt.Errorf("no error found")
	}
	return out, nil
}

// FindFirst returns first result in error that fulfills given errors
func (e *ErrorInspector) FindAll() []*Error {
	out := []*Error{}
	e.findAll(e.rootErr, &out)
	return out
}

// FindCount returns error in case count is exactly the expected count,
// otherwise FindCount returns an error
func (e *ErrorInspector) FindCount(cnt int) ([]*Error, error) {
	out := e.FindAll()
	if len(out) != cnt {
		return nil, fmt.Errorf("expected to find %d errors, got %d", cnt, len(out))
	}
	return out, nil
}

// FindOne expects to find exactly a single error
func (e *ErrorInspector) FindOne() (*Error, error) {
	out := e.FindAll()
	if len(out) != 1 {
		return nil, fmt.Errorf("expected to find exactly one error, got %d", len(out))
	}
	return out[0], nil
}

// Count returns the count of available errors
func (e *ErrorInspector) Count() int {
	out := e.FindAll()
	return len(out)
}

// findFirst finds first matching error
func (e *ErrorInspector) findFirst(err *Error) *Error {
	if e.matches(err) {
		return err
	}
	if err.Data == nil {
		return nil
	}
	for _, v := range err.Data.Details {
		out := e.findFirst(v)
		if out != nil {
			return out
		}
	}
	return nil
}

// findFirst finds first matching error
func (e *ErrorInspector) findAll(err *Error, errs *[]*Error) {
	if e.matches(err) {
		*errs = append(*errs, err)
	}
	if err.Data == nil {
		return
	}
	for _, v := range err.Data.Details {
		e.findAll(v, errs)
	}
}

func (e *ErrorInspector) matches(err *Error) bool {
	if e.code != nil {
		if *e.code != err.Code {
			return false
		}
	}
	if e.message != nil {
		if *e.message != err.Message {
			return false
		}
	}
	if e.path != nil {
		var p []string
		if err.Data != nil {
			p = err.Data.Path
		}
		// in case the length of the paths are not equal,
		// the path cannot be equal
		if len(*e.path) != len(p) {
			return false
		}
		// iterate and compare each path element
		for idx, elem := range *e.path {
			if p[idx] != elem {
				return false
			}
		}
	}
	return true
}
