package jonson

import (
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
		do     func(*AuthProvider) (*string, error)
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
			do: func(p *AuthProvider) (*string, error) {
				public := p.NewPublic(nil)
				out, err := public.AccountUuid(nil)

				// call twice to see call history
				public.AccountUuid(nil)

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
			do: func(p *AuthProvider) (*string, error) {
				public := p.NewPublic(nil)
				out, err := public.AccountUuid(nil)

				// call twice to see call history
				public.AccountUuid(nil)

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
			do: func(p *AuthProvider) (*string, error) {
				public := p.NewPublic(nil)
				out, err := public.AccountUuid(nil)

				// call twice to see call history
				public.AccountUuid(nil)

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
			do: func(p *AuthProvider) (*string, error) {
				private := p.NewPrivate(nil)
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
			do: func(p *AuthProvider) (*string, error) {
				private := p.NewPrivate(nil)
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
				if _, ok := err.(*Error); !ok {
					return fmt.Errorf("expected error to be jonson error, got: %s", err)
				}
				if err.(*Error).Code != ErrUnauthorized.Code {
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
			do: func(p *AuthProvider) (*string, error) {
				private := p.NewPrivate(nil)
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
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			func() {
				// catch the panic
				defer func() {
					var err error
					if _err := recover(); err != nil {
						err = _err.(error)
					}
					v.check(nil, err)
				}()

				authProvider := NewAuthProvider(v.client)
				accountUuid, err := v.do(authProvider)
				v.check(accountUuid, err)

				if v.client.calls != 1 {
					t.Fatal("client was called more than once")
				}
			}()

		})
	}
}
