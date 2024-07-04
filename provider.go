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
