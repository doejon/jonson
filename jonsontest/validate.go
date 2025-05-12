package jonsontest

import (
	"context"

	"github.com/doejon/jonson"
)

// ValidateParams is a convenience function that allows you to write validation tests
// for parameters implementing jonson.ValdiatedParams
func ValidateParams(params jonson.ValidatedParams, ctx ...*jonson.Context) *jonson.Error {
	var c *jonson.Context
	for _, v := range ctx {
		c = v
	}
	if c == nil {
		c = jonson.NewContext(context.Background(), nil, nil)
	}

	return jonson.Validate(c, jonson.NewDebugSecret(), params)
}
