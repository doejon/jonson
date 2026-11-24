package jonson

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

type fieldsPartial struct {
	Params
	Name    string `json:"name"`
	Age     int    `json:"age,omitempty"`
	Tags    []string
	Ignored string `json:"-"`
	hidden  string
}

func (p *fieldsPartial) JonsonValidate(v *Validator) {
	v.Path("name").Check(p.Name != "", "name required")
	if p.Age < 0 { // conditional: unseen on zero value
		v.Path("age").Message("age >= 0")
	}
}

type fieldsFull struct {
	Params
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func (p *fieldsFull) JonsonValidate(v *Validator) {
	v.Path("name").Check(p.Name != "", "name required")
	v.Path("age").Skip()
}

type fieldsPanics struct {
	Params
	Ptr *fieldsFull `json:"ptr"`
}

func (p *fieldsPanics) JonsonValidate(v *Validator) {
	v.Path("ptr").Check(p.Ptr.Age > 0, "nil deref")
}

func TestCheckAndSkip(t *testing.T) {
	ctx := NewContext(context.Background(), NewFactory(), nil)
	err := Validate(ctx, NewDebugSecret(), &fieldsFull{})
	if err == nil || len(err.Data.Details) != 1 || err.Data.Details[0].Message != "name required" {
		t.Fatalf("expected single 'name required' error, got %+v", err)
	}
	if Validate(ctx, NewDebugSecret(), &fieldsFull{Name: "x"}) != nil {
		t.Fatal("expected no error")
	}
}

func TestUnvalidatedFields(t *testing.T) {
	ctx := NewContext(context.Background(), NewFactory(), nil)

	missing, err := unvalidatedFields(ctx, NewDebugSecret(), reflect.TypeFor[fieldsPartial]())
	if err != nil || !slices.Equal(missing, []string{"age", "Tags"}) {
		t.Fatalf("got %v, %v", missing, err)
	}

	missing, err = unvalidatedFields(ctx, NewDebugSecret(), reflect.TypeFor[fieldsFull]())
	if err != nil || len(missing) != 0 {
		t.Fatalf("got %v, %v", missing, err)
	}

	_, err = unvalidatedFields(ctx, NewDebugSecret(), reflect.TypeFor[fieldsPanics]())
	if err == nil || !strings.Contains(err.Error(), "panicked") {
		t.Fatalf("expected panic error, got %v", err)
	}
}

type fieldsSystem struct{}

func (s *fieldsSystem) PartialV1(ctx *Context, p *fieldsPartial) error { return nil }

func TestMissingFieldValidationLevelFatal(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "does not validate fields: age, Tags") {
			t.Fatalf("expected panic, got %v", r)
		}
	}()
	m := NewMethodHandler(NewFactory(), NewDebugSecret(), &MethodHandlerOptions{
		MissingFieldValidationLevel: MissingValidationLevelFatal,
	})
	m.RegisterSystem(&fieldsSystem{})
}
