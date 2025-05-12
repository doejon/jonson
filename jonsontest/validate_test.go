package jonsontest

import (
	"context"
	"testing"
	"time"

	"github.com/doejon/jonson"
)

type TestParam struct {
	FirstName string  `json:"firstName"`
	LastName  string  `json:"lastName"`
	Time      *string `json:"time,omitempty"`
}

func (t *TestParam) JonsonValidate(v *jonson.Validator) {
	if len(t.FirstName) != 6 {
		v.Path("firstName").Message("firstName needs to be 6 characters long")
	}
	if len(t.LastName) != 3 {
		v.Path("lastName").Message("firstName needs to be 3 characters long")
	}
	if t.Time != nil {
		tm := jonson.RequireTime(v.Context)
		t, _ := time.Parse(time.ANSIC, *t.Time)
		if tm.Now().Before(t) {
			v.Path("time").Message("time needs to be before <now>")
		}
	}
}

func TestValidateParams(t *testing.T) {
	validParam := func() *TestParam {
		return &TestParam{
			FirstName: "Silvio",
			LastName:  "Lus",
		}
	}

	type test struct {
		name     string
		ctx      *jonson.Context
		params   func() *TestParam
		validate func(t *testing.T, e *jonson.Error)
	}

	fac := jonson.NewFactory()
	fac.RegisterProvider(jonson.NewTimeProvider(func() jonson.Time {
		return jonson.NewRealTime()
	}))
	ctx := jonson.NewContext(context.Background(), fac, nil)

	tests := []*test{
		{
			name: "valid params",
			params: func() *TestParam {
				out := validParam()
				return out
			},
			validate: func(t *testing.T, e *jonson.Error) {
				if e != nil {
					t.Fatal("expect error to be nil")
				}
			},
		},
		{
			name: "valid params",
			params: func() *TestParam {
				out := validParam()
				out.FirstName = ""
				return out
			},
			validate: func(t *testing.T, e *jonson.Error) {
				if e.Inspect().Count() != 2 {
					t.Fatal("expected to have two errors")
				}
			},
		},
		{
			name: "valid params",
			params: func() *TestParam {
				out := validParam()
				out.FirstName = ""
				out.LastName = ""
				return out
			},
			validate: func(t *testing.T, e *jonson.Error) {
				if e.Inspect().Count() != 3 {
					t.Fatal("expected to have 3 errors")
				}
			},
		},
		{
			name: "valid time",
			params: func() *TestParam {
				out := validParam()
				ansic := time.ANSIC
				out.Time = &ansic
				return out
			},
			ctx: ctx,
			validate: func(t *testing.T, e *jonson.Error) {
				if e != nil {
					t.Fatal("expect error to be nil")
				}
			},
		},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			params := v.params()
			err := ValidateParams(params, v.ctx)
			v.validate(t, err)
		})
	}
}
