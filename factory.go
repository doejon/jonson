package jonson

import (
	"errors"
	"reflect"
	"strings"
)

type Factory struct {
	providers map[reflect.Type]boundMethod
}

type boundMethod struct {
	this   reflect.Value
	method reflect.Value
}

func NewFactory() *Factory {
	return &Factory{
		providers: map[reflect.Type]boundMethod{},
	}
}

func (f *Factory) Provide(ctx *Context, rt reflect.Type) interface{} {
	bm, exists := f.providers[rt]
	if !exists {
		panic(errors.New("factory: unknown provider type requested: " + rt.String()))
	}

	refctx := reflect.ValueOf(ctx)

	var pres []reflect.Value

	if !bm.this.IsValid() || bm.this.IsNil() {
		pres = bm.method.Call([]reflect.Value{refctx})
	} else {
		pres = bm.method.Call([]reflect.Value{bm.this, refctx})
	}

	return pres[0].Interface()
}

func (f *Factory) RegisterProviderFunc(fn interface{}) {
	rtfn := reflect.TypeOf(fn)
	if rtfn.Kind() != reflect.Func {
		panic(errors.New("factory: expect registered function to be a function"))
	}

	// input
	if rtfn.NumIn() != 1 {
		panic(errors.New("factory: expect registered function to have exactly 1 argument"))
	}
	if rtfn.In(0) != TypeContext {
		panic(errors.New("factory: expect registered function to have *jonson.Context as first argument"))
	}

	// output
	if rtfn.NumOut() != 1 {
		panic(errors.New("factory: expect registered function to have exactly 1 return value"))
	}
	rtfno := rtfn.Out(0)
	if rtfno.Kind() != reflect.Interface && (rtfno.Kind() != reflect.Ptr || rtfno.Elem().Kind() != reflect.Struct) {
		panic(errors.New("factory: expect return type to be an interface or ptr to struct"))
	}

	f.providers[rtfno] = boundMethod{
		method: reflect.ValueOf(fn),
	}
}

// RegisterProvider registers a new Provider and panics on error
func (f *Factory) RegisterProvider(provider interface{}) {
	// step 1 - check if we have a ptr to a struct
	rv := reflect.ValueOf(provider)
	rt := reflect.TypeOf(provider)
	if rt.Kind() != reflect.Ptr || rt.Elem().Kind() != reflect.Struct {
		panic(errors.New("factory: must pass ptr to struct"))
	}

	// step 2 - scan methods
	for i := 0; i < rt.NumMethod(); i++ {
		rtm := rt.Method(i)

		// only map methods that start with New
		if !strings.HasPrefix(rtm.Name, "New") {
			continue
		}

		// check if we have the following argument signature:
		// (this $this, ctx *Context)

		// check input
		if rtm.Type.NumIn() != 2 {
			panic(errors.New("factory: expect " + rtm.Name + " to have exactly 1 argument"))
		}
		if rtm.Type.In(1) != TypeContext {
			panic(errors.New("factory: expect " + rtm.Name + " to have *jonson.Context as first argument"))
		}

		// check output
		if rtm.Type.NumOut() != 1 {
			panic(errors.New("factory: expect " + rtm.Name + " to have exactly 1 return value"))
		}
		if rtm.Type.Out(0).Kind() != reflect.Interface &&
			(rtm.Type.Out(0).Kind() != reflect.Ptr || rtm.Type.Out(0).Elem().Kind() != reflect.Struct) {
			panic(errors.New("factory: expect " + rtm.Name + " to have an interface or ptr to struct as first return value"))
		}

		// register provider
		t := rtm.Type.Out(0)
		if _, exists := f.providers[t]; exists {
			panic(errors.New("factory: provider for type " + t.String() + " already exists"))
		}
		f.providers[t] = boundMethod{
			this:   rv,
			method: rtm.Func,
		}
	}
}

func (f *Factory) Types() []reflect.Type {
	res := make([]reflect.Type, 0, len(f.providers))
	for rt := range f.providers {
		res = append(res, rt)
	}
	return res
}
