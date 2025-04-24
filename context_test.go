package jonson

import (
	"context"
	"testing"
)

func TestRootContext(t *testing.T) {

	type test struct {
		name    string
		root    context.Context
		context func() *Context
	}

	tests := []*test{
		{
			name: "nil ctx",
			root: nil,
			context: func() *Context {
				return NewContext(nil, nil, nil)
			},
		},
		{
			name: "TODO ctx",
			root: context.TODO(),
			context: func() *Context {
				return NewContext(context.TODO(), nil, nil)
			},
		},
		{
			name: "background ctx",
			root: context.Background(),
			context: func() *Context {
				return NewContext(context.Background(), nil, nil)
			},
		},

		{
			name: "background ctx, cloned",
			root: context.Background(),
			context: func() *Context {
				return NewContext(context.Background(), nil, nil).Clone()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			out := test.context().RootContext()
			if out != test.root {
				t.Fatal("expected root context to equal generated context")
			}
		})
	}

}

func TestClone(t *testing.T) {

	t.Run("a cloned context is not the same as the original context", func(t *testing.T) {
		ctx := NewContext(context.TODO(), nil, nil)
		cloned := ctx.Clone()

		if ctx == cloned {
			t.Fatal("cloned must not be the same as context")
		}
	})

}
