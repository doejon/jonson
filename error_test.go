package jonson

import "testing"

func TestError(t *testing.T) {
	testError := &Error{
		Code:    1,
		Message: "error 1",
		Data: &ErrorData{
			Path: []string{"first", "second"},
			Details: []*Error{
				{
					Code:    2,
					Message: "error 2",
					Data: &ErrorData{
						Path: []string{"third", "fourth"},
						Details: []*Error{
							{
								Code:    2,
								Message: "error 2",
							},
						},
					},
				},
				{
					Code:    3,
					Message: "error 3",
				},
			},
		},
	}

	type test struct {
		name     string
		validate func(t *testing.T, e *ErrorInspector)
	}

	tests := []*test{
		{
			name: "correct count is returned",
			validate: func(t *testing.T, e *ErrorInspector) {
				if e.Count() != 4 {
					t.Fatalf("expected 4 errors, got: %d", e.Count())
				}
			},
		},
		{
			name: "correct count is returned when filtered for code 1",
			validate: func(t *testing.T, e *ErrorInspector) {
				if e.Code(1).Count() != 1 {
					t.Fatalf("expected 1 errors, got: %d", e.Count())
				}
			},
		},
		{
			name: "correct count is returned when filtered for code 2",
			validate: func(t *testing.T, e *ErrorInspector) {
				if e.Code(2).Count() != 2 {
					t.Fatalf("expected 2 errors, got: %d", e.Count())
				}
			},
		},
		{
			name: "correct errors are returned when searching for code 2",
			validate: func(t *testing.T, e *ErrorInspector) {
				errors := e.Code(2).FindAll()
				if len(errors) != 2 {
					t.Fatalf("expected 2 errors, got: %d", e.Count())
				}
				if errors[0].Code != 2 || errors[1].Code != 2 {
					t.Fatalf("expected code of returned errors to equal 2")
				}
			},
		},
		{
			name: "correct errors are returned when searching for code 2",
			validate: func(t *testing.T, e *ErrorInspector) {
				errors := e.Message("error 1").FindAll()
				if len(errors) != 1 {
					t.Fatalf("expected 1 error, got: %d", e.Count())
				}
				if errors[0].Message != "error 1" {
					t.Fatalf("expected message to match, got: %s", errors[0].Message)
				}
			},
		},
		{
			name: "findFirst returns first error",
			validate: func(t *testing.T, e *ErrorInspector) {
				foundErr, err := e.Message("error 1").FindFirst()
				if err != nil {
					t.Fatal("expected to find error by message")
				}
				if foundErr.Code != 1 {
					t.Fatal("expected correct error to be returned")
				}
			},
		},
		{
			name: "findFirst returns first error - even when there are multiple available",
			validate: func(t *testing.T, e *ErrorInspector) {
				foundErr, err := e.Code(2).FindFirst()
				if err != nil {
					t.Fatal("expected to find error by code")
				}
				if foundErr.Code != 2 {
					t.Fatal("expected correct error to be returned")
				}
			},
		},
		{
			name: "findFirst returns error in case none can be found",
			validate: func(t *testing.T, e *ErrorInspector) {
				_, err := e.Code(4).FindFirst()
				if err == nil {
					t.Fatal("expected to find no error by code")
				}
			},
		},
		{
			name: "findOne returns a single error",
			validate: func(t *testing.T, e *ErrorInspector) {
				foundErr, err := e.Code(1).FindOne()
				if err != nil {
					t.Fatal("expected to find error by code")
				}
				if foundErr.Code != 1 {
					t.Fatal("expected correct error to be returned")
				}
			},
		},
		{
			name: "findOne fails due to multiple errors being present",
			validate: func(t *testing.T, e *ErrorInspector) {
				_, err := e.Code(2).FindOne()
				if err == nil {
					t.Fatal("expected error, multiple exist")
				}
			},
		},
		{
			name: "findCount returns expected count",
			validate: func(t *testing.T, e *ErrorInspector) {
				out, err := e.Code(2).FindCount(2)
				if err != nil {
					t.Fatal("expected no error")
				}
				if len(out) != 2 {
					t.Fatalf("expected len 2, got %d", len(out))
				}
				if out[0].Code != 2 || out[1].Code != 2 {
					t.Fatalf("expected codes to equal 2")
				}
			},
		},
		{
			name: "findCount returns error in case count does not match",
			validate: func(t *testing.T, e *ErrorInspector) {
				_, err := e.Code(2).FindCount(1)
				if err == nil {
					t.Fatal("expected error")
				}
			},
		},
		{
			name: "finding error by path succeeds",
			validate: func(t *testing.T, e *ErrorInspector) {
				out, err := e.Path("first", "second").FindOne()
				if err != nil {
					t.Fatal("expected no error")
				}
				if out.Code != 1 {
					t.Fatalf("expected error code 1, got %d", out.Code)
				}
			},
		},
		{
			name: "finding error by unknown path fails",
			validate: func(t *testing.T, e *ErrorInspector) {
				_, err := e.Path("first").FindOne()
				if err == nil {
					t.Fatal("expected error")
				}
			},
		},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			inspector := NewErrorInspector(testError)
			v.validate(t, inspector)
		})
	}
}
