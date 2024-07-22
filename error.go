package jonson

import (
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

// ErrorData object
type ErrorData struct {
	Path    []string `json:"path,omitempty"`
	Details []*Error `json:"details,omitempty"`
	Debug   string   `json:"debug,omitempty"`
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

// CloneWithData returns a copy with the supplied data set
func (e Error) CloneWithData(data *ErrorData) *Error {
	e.Data = data
	return &e
}

func (e Error) String() string {
	data := ""
	if e.Data != nil {
		data = e.Data.String()
	}
	return fmt.Sprintf("error code: %d | message: %s | data: %s", e.Code, e.Message, data)
}
