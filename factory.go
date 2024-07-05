package jonson

import (
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

// Provide provides a registered type.
// The returned type is of type rt.
func (f *Factory) Provide(ctx *Context, rt reflect.Type) any {
	bm, exists := f.providers[rt]
	if !exists {
		panic("factory: unknown provider type requested: " + rt.String())
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

// RegisterProviderFunc allows us to register a single function returning a provider.
// Example:
//
//	 func ProvideDB(ctx *jonson.Context)*sql.DB{
//		  return &sql.NewDB()
//	 }
//
//	 fac := jonson.NewFactory()
//	 fac.RegisterProviderFunc(ProvideDB)
func (f *Factory) RegisterProviderFunc(fn any) {
	rtfn := reflect.TypeOf(fn)
	if rtfn.Kind() != reflect.Func {
		panic("factory: expect registered function to be a function")
	}

	// input
	if rtfn.NumIn() != 1 {
		panic("factory: expect registered function to have exactly 1 argument")
	}
	if rtfn.In(0) != TypeContext {
		panic("factory: expect registered function to have *jonson.Context as first argument")
	}

	// output
	if rtfn.NumOut() != 1 {
		panic("factory: expect registered function to have exactly 1 return value")
	}
	rtfno := rtfn.Out(0)
	if rtfno.Kind() != reflect.Interface && (rtfno.Kind() != reflect.Ptr || rtfno.Elem().Kind() != reflect.Struct) {
		panic("factory: expect return type to be an interface or ptr to struct")
	}

	f.providers[rtfno] = boundMethod{
		method: reflect.ValueOf(fn),
	}
}

// RegisterProvider registers a new Provider and panics on error
// The provider needs to be a pointer to a struct which provides
// methods accepting *jonson.Context and returning a single type.
// The method's name needs to be equal to the returned type's name
// and start with New.
// Example:
//
//	type Provider struct {}
//
//	type Time struct {}
//
//	func (p *Provider) NewTime() *Time{
//		return &Time{}
//	}
//
//	type DB struct {}
//
//	func(p *Provider) NewDB() *DB {
//		return &DB{}
//	}
//
//	fac := jonson.NewFactory()
//	fac.RegisterProvider(&Provider{})
func (f *Factory) RegisterProvider(provider any) {
	// step 1 - check if we have a ptr to a struct
	rv := reflect.ValueOf(provider)
	rt := reflect.TypeOf(provider)
	if rt.Kind() != reflect.Ptr || rt.Elem().Kind() != reflect.Struct {
		panic("factory: must pass ptr to struct")
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
			panic("factory: expect " + rtm.Name + " to have exactly 1 argument")
		}
		if rtm.Type.In(1) != TypeContext {
			panic("factory: expect " + rtm.Name + " to have *jonson.Context as first argument")
		}

		// check output
		if rtm.Type.NumOut() != 1 {
			panic("factory: expect " + rtm.Name + " to have exactly 1 return value")
		}
		if rtm.Type.Out(0).Kind() != reflect.Interface &&
			(rtm.Type.Out(0).Kind() != reflect.Ptr || rtm.Type.Out(0).Elem().Kind() != reflect.Struct) {
			panic("factory: expect " + rtm.Name + " to have an interface or ptr to struct as first return value")
		}

		// register provider
		t := rtm.Type.Out(0)
		if _, exists := f.providers[t]; exists {
			panic("factory: provider for type " + t.String() + " already exists")
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
