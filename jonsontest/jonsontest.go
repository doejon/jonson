package jonsontest

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"testing"

	"github.com/doejon/jonson"
)

type TestContextBoundary struct {
	opts          []NewTestContextBoundaryOpt
	methodHandler *jonson.MethodHandler
	factory       *jonson.Factory
	t             *testing.T

	stackInspector []func(s string)
}

// NewTestContext returns a new test context
func NewContextBoundary(
	t *testing.T,
	factory *jonson.Factory,
	methodHandler *jonson.MethodHandler,
	opts ...NewTestContextBoundaryOpt) *TestContextBoundary {
	// set default opts
	return &TestContextBoundary{
		factory:       factory,
		methodHandler: methodHandler,
		opts:          append([]NewTestContextBoundaryOpt{}, opts...),
		t:             t,
	}
}

// Run runs test. In case you need to inspect a panic,
// handle a recover fn (recovr) which will receive the stack as a fn argument
func (t *TestContextBoundary) Run(fn func(ctx *jonson.Context) error, recovr ...func(stack string)) (err error) {
	ctx := jonson.NewContext(
		context.Background(),
		t.factory,
		t.methodHandler,
	)
	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			inspectors := append(t.stackInspector, recovr...)
			for _, v := range inspectors {
				v(stack)
			}
			err = getRecoverError(r)
		}
		err = ctx.Finalize(err)
	}()
	for _, opt := range t.opts {
		opt(ctx)
	}
	return fn(ctx)
}

// MustRun makes the parent test fail in case of an error
func (t *TestContextBoundary) MustRun(fn func(ctx *jonson.Context) error, recovr ...func(stack string)) {
	err := t.Run(fn, recovr...)

	t.t.Helper()
	if err != nil {
		t.t.Fatal(err)
	}
}

// WithStackInspector allows you to specify a stack inspector which will be enabled for
// any of the run calls. You can optionally also pass a second argument to Run() or MustRun()
func (t *TestContextBoundary) WithStackInspector(recovr ...func(stack string)) *TestContextBoundary {
	t.stackInspector = append(t.stackInspector, recovr...)
	return t
}

type NewTestContextBoundaryOpt func(*jonson.Context)

// WithHttpSource provides an http source to the context
func (t *TestContextBoundary) WithHttpSource(r *http.Request, w http.ResponseWriter) *TestContextBoundary {
	t.opts = append(t.opts, WithHttpSource(r, w))
	return t
}

// WithRpcMeta provides rpc meta to the context
func (t *TestContextBoundary) WithRpcMeta(meta *jonson.RpcMeta) *TestContextBoundary {
	t.opts = append(t.opts, WithRpcMeta(meta))
	return t
}

// WithHttpSource allows us to create a new http source for the test context boundary
func WithHttpSource(r *http.Request, w http.ResponseWriter) NewTestContextBoundaryOpt {
	return func(ctx *jonson.Context) {
		ctx.StoreValue(jonson.TypeHttpRequest, &jonson.HttpRequest{
			Request: r,
		})
		ctx.StoreValue(jonson.TypeHttpResponseWriter, &jonson.HttpResponseWriter{
			ResponseWriter: w,
		})
	}
}

// WithRpcMeta allows us to provide rpc meta to the test context boundary
func WithRpcMeta(meta *jonson.RpcMeta) NewTestContextBoundaryOpt {
	return func(ctx *jonson.Context) {
		ctx.StoreValue(jonson.TypeRpcMeta, meta)
	}
}

func getRecoverError(e any) error {
	err, ok := e.(error)
	if ok {
		return err
	}
	s, ok := e.(string)
	if ok {
		return errors.New(s)
	}
	return fmt.Errorf("%v", e)
}
