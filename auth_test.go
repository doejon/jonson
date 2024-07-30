package jonson

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

const testAccountUuid = "e315a894-c76a-49d5-a682-bf909bd48d32"

type testAuthClient struct {
	isAuthorized    bool
	isAuthorizedErr error

	isAuthenticated    bool
	isAuthenticatedErr error

	calls int
}

func (t *testAuthClient) IsAuthenticated(*Context) (*string, error) {
	t.calls++
	cpy := testAccountUuid
	if t.isAuthenticated {
		return &cpy, t.isAuthenticatedErr
	}
	return nil, t.isAuthenticatedErr
}

func (t *testAuthClient) IsAuthorized(*Context) (*string, error) {
	t.calls++
	cpy := testAccountUuid
	if t.isAuthorized {
		return &cpy, t.isAuthorizedErr
	}
	return nil, t.isAuthorizedErr
}

func TestAuthentication(t *testing.T) {

	type test struct {
		name   string
		client *testAuthClient
		do     func(*Context, *AuthProvider) (*string, error)
		check  func(accountUuid *string, err error) error
	}

	tests := []*test{
		{
			name: "authenticated",
			client: &testAuthClient{
				isAuthenticated:    true,
				isAuthenticatedErr: nil,
				isAuthorized:       false,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				public := p.NewPublic(ctx)
				out, err := public.AccountUuid(ctx)

				// call twice to see call history
				public.AccountUuid(ctx)

				return out, err
			},
			check: func(accountUuid *string, err error) error {
				if accountUuid == nil {
					return errors.New("expected account uuid not to be nil")
				}
				if *accountUuid != testAccountUuid {
					return errors.New("expected test account uuid to match")
				}
				if err != nil {
					return fmt.Errorf("expected error to be nil, got: %s", err)
				}
				return nil
			},
		},
		{
			name: "not authenticated, no err",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: nil,
				isAuthorized:       false,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				public := p.NewPublic(ctx)
				out, err := public.AccountUuid(ctx)

				// call twice to see call history
				public.AccountUuid(ctx)

				return out, err
			},
			check: func(accountUuid *string, err error) error {
				if accountUuid != nil {
					return errors.New("expected no account uuid")
				}
				if err != nil {
					return fmt.Errorf("expected error to be nil, got: %s", err)
				}
				return nil
			},
		},
		{
			name: "not authenticated, err",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: errors.New("failed to check session"),
				isAuthorized:       false,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				public := p.NewPublic(ctx)
				out, err := public.AccountUuid(ctx)

				// call twice to see call history
				public.AccountUuid(ctx)

				return out, err
			},
			check: func(accountUuid *string, err error) error {
				if accountUuid != nil {
					return errors.New("expected no account uuid")
				}
				if err == nil {
					return errors.New("expected error, got nil")
				}
				return nil
			},
		},
		{
			name: "authorized",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: nil,
				isAuthorized:       true,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				private := p.NewPrivate(ctx)
				out := private.AccountUuid()
				return &out, nil
			},
			check: func(accountUuid *string, err error) error {
				if accountUuid == nil {
					return errors.New("expected account uuid not to be nil")
				}
				if *accountUuid != testAccountUuid {
					return errors.New("expected test account uuid to match")
				}
				if err != nil {
					return fmt.Errorf("expected error to be nil, got: %s", err)
				}
				return nil
			},
		},
		{
			name: "not authorized, no err",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: nil,
				isAuthorized:       false,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				private := p.NewPrivate(ctx)
				out := private.AccountUuid()
				return &out, nil
			},
			check: func(accountUuid *string, err error) error {
				if accountUuid != nil {
					return errors.New("expected no account uuid")
				}
				if err != nil {
					return fmt.Errorf("expected error to be nil, got: %s", err)
				}
				casted, ok := err.(*Error)
				if !ok {
					return fmt.Errorf("expected error to be jonson error, got: %s", err)
				}
				if casted.Code != ErrUnauthorized.Code {
					return fmt.Errorf("error code should match unauthorized")
				}
				return nil
			},
		},
		{
			name: "not authorized, err",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: nil,
				isAuthorized:       false,
				isAuthorizedErr:    errors.New("failed to check session"),
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				private := p.NewPrivate(ctx)
				out := private.AccountUuid()
				return &out, nil
			},
			check: func(accountUuid *string, err error) error {
				if accountUuid != nil {
					return errors.New("expected no account uuid")
				}
				if err == nil {
					return errors.New("expected error, got nil")
				}
				return nil
			},
		},
		{
			name: "authenticated fails since user is not authenticated and private was not called before",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: errors.New("not authenticated"),
				isAuthorized:       true,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				return RequirePublic(ctx).AccountUuid(ctx)
			},
			check: func(accountUuid *string, err error) error {
				if err == nil {
					t.Fatalf("expected no error, got: %s", err)
				}
				if err.Error() != "not authenticated" {
					t.Fatalf("expected error to equal 'not authenticated', got: %s", err)
				}
				if accountUuid != nil {
					return errors.New("expected no account uuid")
				}
				return nil
			},
		},
		// the previous test makes sure the account uuid is not resolved in case
		// the user is not logged in;
		// here, we check for the account uuid that has previously been
		// resolved by a call to NewPrivate(): we know that the account is logged in
		// and do not need to call the remote client once more
		{
			name: "authorized and call to public will resolve the same uuid",
			client: &testAuthClient{
				isAuthenticated:    false,
				isAuthenticatedErr: errors.New("not authenticated"),
				isAuthorized:       true,
				isAuthorizedErr:    nil,
			},
			do: func(ctx *Context, p *AuthProvider) (*string, error) {
				RequirePrivate(ctx)
				out, _ := RequirePublic(ctx).AccountUuid(ctx)

				return out, nil
			},
			check: func(accountUuid *string, err error) error {
				if err != nil {
					t.Fatalf("expected no error, got: %s", err)
				}
				if accountUuid == nil {
					return errors.New("expected account uuid")
				}
				if *accountUuid != testAccountUuid {
					t.Fatalf("expected account uuid to match: %s | %s", *accountUuid, testAccountUuid)
				}
				return nil
			},
		},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			func() {
				// catch the panic
				defer func() {
					err := recover()
					if err == nil {
						return
					}
					if s, ok := err.(string); ok {
						err = errors.New(s)
					}
					v.check(nil, err.(error))
				}()

				fac := NewFactory()
				authProvider := NewAuthProvider(v.client)
				fac.RegisterProvider(authProvider)
				ctx := NewContext(context.Background(), fac, nil)

				accountUuid, err := v.do(ctx, authProvider)
				v.check(accountUuid, err)

				if v.client.calls != 1 {
					t.Fatal("client was called more than once")
				}
			}()

		})
	}
}
