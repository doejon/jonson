package jonson

import (
	"net/url"

	"github.com/gorilla/schema"
)

// formValuesDecoder converts the string values used by forms and URL queries
// into the concrete Go types declared by an endpoint's params struct. Keeping
// this conversion schema-aware avoids guessing a value's type from its text.
type formValuesDecoder struct {
	strict       *schema.Decoder
	allowUnknown *schema.Decoder
}

func newFormValuesDecoder() *formValuesDecoder {
	// Endpoint params already use json tags as their public field names. Reuse
	// those tags so supporting another transport does not require duplicate tags.
	strict := schema.NewDecoder()
	strict.SetAliasTag("json")

	// Match the JsonHandler behavior: unknown fields are rejected unless the
	// params type explicitly opts out through AllowUnknownFieldsParams.
	allowUnknown := schema.NewDecoder()
	allowUnknown.SetAliasTag("json")
	allowUnknown.IgnoreUnknownKeys(true)

	return &formValuesDecoder{
		strict:       strict,
		allowUnknown: allowUnknown,
	}
}

func (d *formValuesDecoder) Decode(values url.Values, out any) error {
	if _, ok := out.(AllowUnknownFieldsParams); ok {
		return d.allowUnknown.Decode(out, values)
	}
	return d.strict.Decode(out, values)
}
