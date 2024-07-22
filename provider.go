package jonson

import (
	"reflect"
)

type Provider interface {
	Provide(ctx *Context, rt reflect.Type) any
	Types() []reflect.Type
}

func isTypeSupported(list []reflect.Type, rt reflect.Type) bool {
	for i := range list {
		if list[i] == rt {
			return true
		}
	}
	return false
}

// Shareable defines objects that can be shared between contexts
// and will be passed to new contexts created within existing contexts.
// In case you do have a provided method that needs to be forwarded to new contexts
// created in the current scope, mark them as Shareable:
//
//	type Time struct {
//	  jonson.Shareable
//	  time.Time
//	}
type Shareable interface {
	_isShareable()
}
