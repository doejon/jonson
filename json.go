package jonson

import (
	"encoding/json"
	"io"
	"reflect"
)

type jsonHandlerProvider struct {
	jsonHandler JsonHandler
}

func newJsonHandlerProvider(handler JsonHandler) *jsonHandlerProvider {
	return &jsonHandlerProvider{
		jsonHandler: handler,
	}
}

func (l *jsonHandlerProvider) NewJsonHandler(ctx *Context) JsonHandler {
	return l.jsonHandler
}

var TypeJsonHandler = reflect.TypeOf((*JsonHandler)(nil)).Elem()

// RequireLogger allows you to require the logger provided
// during initialization.
// In case no logger was provided, a NoOpLogger will be returned.
// The logger will be available by default.
func RequireJsonHandler(ctx *Context) JsonHandler {
	out := ctx.Require(TypeJsonHandler)
	return out.(JsonHandler)
}

type JsonHandler interface {
	Marshal(data any) ([]byte, error)
	Unmarshal(b []byte, out any) error
	NewEncoder(w io.Writer) JsonEncoder
	NewDecoder(w io.Reader) JsonDecoder
}

type JsonEncoder interface {
	Encode(v any) error
}

type JsonDecoder interface {
	Decode(v any) error
}

type DefaultJsonHandler struct {
}

var _ JsonHandler = &DefaultJsonHandler{}

func NewDefaultJsonHandler() *DefaultJsonHandler {
	return &DefaultJsonHandler{}
}

func (d *DefaultJsonHandler) Marshal(data any) ([]byte, error) {
	return json.Marshal(data)
}

func (d *DefaultJsonHandler) Unmarshal(b []byte, out any) error {
	return json.Unmarshal(b, out)
}

type jsonEncoder struct {
	d *json.Encoder
}

func (d *jsonEncoder) Encode(v any) error {
	return d.d.Encode(v)
}

func (d *DefaultJsonHandler) NewEncoder(w io.Writer) JsonEncoder {
	return &jsonEncoder{
		d: json.NewEncoder(w),
	}
}

type jsonDecoder struct {
	d *json.Decoder
}

func (d *jsonDecoder) Decode(v any) error {
	// in case the param allows unknown fields, we allow
	// for parsing unknown fields -> otherwise not
	_, allowUnknownFields := v.(AllowUnknownFieldsParams)
	if !allowUnknownFields {
		d.d.DisallowUnknownFields()
	}
	d.d.UseNumber()

	return d.d.Decode(v)
}

func (d *DefaultJsonHandler) NewDecoder(r io.Reader) JsonDecoder {
	return &jsonDecoder{
		d: json.NewDecoder(r),
	}
}
