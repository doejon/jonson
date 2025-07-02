package plugins

import (
	"io"

	"github.com/doejon/jonson"
)

type JsonEncodeMutator interface {
	MutateEncode(d any)
}

type JsonDecodeMutator interface {
	MutateDecode(d any)
}

type JsonMutatorHandler struct {
	json          jonson.JsonHandler
	encodeMutator []JsonEncodeMutator
	decodeMutator []JsonDecodeMutator
}

var _ jonson.JsonHandler = (&JsonMutatorHandler{})

// NewJsonMutatorHandler returns a a new json handler that
// will allow you to define mutators before encoding/decoding
// with a provided json handler.
func NewJsonMutatorHandler(handler jonson.JsonHandler) *JsonMutatorHandler {
	out := &JsonMutatorHandler{
		json: handler,
	}

	return out
}

func (d *JsonMutatorHandler) WithEncodeMutator(m JsonEncodeMutator) *JsonMutatorHandler {
	d.encodeMutator = append(d.encodeMutator, m)
	return d
}

func (d *JsonMutatorHandler) WithDecodeMutator(m JsonDecodeMutator) *JsonMutatorHandler {
	d.decodeMutator = append(d.decodeMutator, m)
	return d
}

func (d *JsonMutatorHandler) mutateEncode(data any) {
	for _, v := range d.encodeMutator {
		v.MutateEncode(data)
	}
}

func (d *JsonMutatorHandler) mutateDecode(data any) {
	for _, v := range d.decodeMutator {
		v.MutateDecode(data)
	}
}

func (d *JsonMutatorHandler) Marshal(data any) ([]byte, error) {
	d.mutateEncode(data)
	return d.json.Marshal(data)
}

func (d *JsonMutatorHandler) Unmarshal(b []byte, out any) error {
	err := d.json.Unmarshal(b, out)
	if err != nil {
		return err
	}
	d.mutateDecode(out)
	return nil
}

type jsonNilSliceEncoder struct {
	d       jonson.JsonEncoder
	handler *JsonMutatorHandler
}

func (d *jsonNilSliceEncoder) Encode(v any) error {
	d.handler.mutateEncode(v)
	return d.d.Encode(v)
}

func (d *JsonMutatorHandler) NewEncoder(w io.Writer) jonson.JsonEncoder {
	return &jsonNilSliceEncoder{
		handler: d,
		d:       d.json.NewEncoder(w),
	}
}

type jsonNilSliceDecoder struct {
	d       jonson.JsonDecoder
	handler *JsonMutatorHandler
}

func (d *jsonNilSliceDecoder) Decode(v any) error {
	err := d.d.Decode(v)
	if err != nil {
		return err
	}
	d.handler.mutateDecode(v)
	return nil
}

func (d *JsonMutatorHandler) NewDecoder(r io.Reader) jonson.JsonDecoder {
	out := &jsonNilSliceDecoder{
		d:       d.json.NewDecoder(r),
		handler: d,
	}
	return out
}
