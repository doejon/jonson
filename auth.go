package jonson

import (
	"fmt"
	"reflect"
	"sync"
)

// AuthProvider allows us to enable authentication
// within our calls.
type AuthProvider struct {
	client AuthClient
}

// AuthClient can be implemented by any
// backend which can check for IsAuthenticated or IsAuthorized.
type AuthClient interface {

	// IsAuthenticated: does the caller possess a valid session - hence do we know who them is?
	// In case an error occurs (networking issues or others), IsAuthorized should return (nil, err);
	// In case of a missing authentication, the function should return (nil, nil)
	// In case of a valid authentication, the function should return (account's uuid, nil)
	IsAuthenticated(ctx *Context) (*string, error)
	// IsAuthorized: does the caller possess a valid session _and_ cann the caller access the current method?
	// In case an error occurs (networking issues or others), IsAuthorized should return (nil, err);
	// In case of a missing authorization, the function should return (nil, nil)
	// In case of a valid authorization, the function should return (account's uuid, nil)
	IsAuthorized(ctx *Context) (*string, error)
}

// NewAuthProvider returns a new instance of an auth provider
func NewAuthProvider(
	client AuthClient,
) *AuthProvider {
	return &AuthProvider{
		client: client,
	}
}

// Private references endpoints which are private
type Private struct {
	accountUuid string
}

var TypePrivate = reflect.TypeOf((**Private)(nil)).Elem()

func RequirePrivate(ctx *Context) *Private {
	if v := ctx.Require(TypePrivate); v != nil {
		return v.(*Private)
	}
	return nil
}

func (p *Private) AccountUuid() string {
	return p.accountUuid
}

// Public references endpoints which are public
type Public struct {
	checked     bool
	accountUuid *string
	err         error
	client      AuthClient

	mux sync.Mutex
}

var TypePublic = reflect.TypeOf((**Public)(nil)).Elem()

// RequirePublic returns a public caller
func RequirePublic(ctx *Context) *Public {
	if v := ctx.Require(TypePublic); v != nil {
		return v.(*Public)
	}
	return nil
}

// AccountUuid gets the underlying account uuid.
// The call towards AccountUuid is protected with a mutex:
// in case two callers try to access the account uuid at the same time,
// only one will do the request;
// Possible return values:
// nil, err --> the client had an error
// nil, nil --> no error, not authenticated
// account uuid, nil -> no error, authenticated
func (p *Public) AccountUuid(ctx *Context) (*string, error) {
	// protect the call against concurrent calls
	p.mux.Lock()
	if p.checked {
		// we're done fetching, no need to lock any longer
		p.mux.Unlock()
		if p.accountUuid == nil {
			return nil, p.err
		}
		out := *p.accountUuid
		return &out, nil
	}

	// unlock the mutex and mark
	// the current session as checked
	defer func() {
		p.checked = true
		p.mux.Unlock()
	}()

	// call the client and keep the response
	// in memory
	resp, err := p.client.IsAuthenticated(ctx)
	p.accountUuid = resp
	p.err = err

	var id *string
	if resp != nil {
		cpy := *resp
		id = &cpy
	}
	return id, err
}

// NewPrivate returns a new private instance
func (p *AuthProvider) NewPrivate(ctx *Context) *Private {
	resp, err := p.client.IsAuthorized(ctx)
	if err != nil {
		panic(fmt.Sprintf("newPrivate: %s", err))
	}
	if resp == nil {
		panic(ErrUnauthorized)
	}

	return &Private{
		accountUuid: *resp,
	}
}

// NewPublic returns a new public instance
func (p *AuthProvider) NewPublic(ctx *Context) *Public {
	return &Public{
		client:      p.client,
		checked:     false,
		accountUuid: nil,
	}
}
