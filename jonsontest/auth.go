package jonsontest

import (
	"reflect"

	"github.com/doejon/jonson"
)

type AuthClientMock struct {
	// method access:  map[accountUuid]
	methodAccess map[string][]*RpcMethod

	// full access:  map[accountUuid]
	fullAccess map[string]struct{}

	// logged ins:  map[accountUuid]
	authenticated map[string]struct{}
}

func NewAuthClientMock() *AuthClientMock {
	return &AuthClientMock{
		methodAccess:  map[string][]*RpcMethod{},
		fullAccess:    map[string]struct{}{},
		authenticated: map[string]struct{}{},
	}
}

var _ jonson.AuthClient = (&AuthClientMock{})

type Account struct {
	uuid string // the secret that will identify the account during tests
	mock *AuthClientMock
}

var typeTestAccount = reflect.TypeOf((**Account)(nil)).Elem()

func (a *Account) Provide(ctx *jonson.Context) {
	ctx.StoreValue(typeTestAccount, a)
}

func (a *Account) Authenticated() *Account {
	a.mock.authenticated[a.uuid] = struct{}{}
	return a
}

type RpcMethod struct {
	HttpMethod jonson.RpcHttpMethod
	Method     string
}

// Authorized allows the account to be authorized.
// In case no methods are provided,the account will be authorized to call
// all methods, otherwise just those provided
func (a *Account) Authorized(methods ...*RpcMethod) *Account {
	// make sure the account is also authenticated
	// since authorized accounts are also logged in
	a.Authenticated()

	if len(methods) <= 0 {
		a.mock.fullAccess[a.uuid] = struct{}{}
		return a
	}

	for _, v := range methods {
		existing := a.mock.methodAccess[a.uuid]
		existing = append(existing, v)
		a.mock.methodAccess[a.uuid] = existing
	}
	return a
}

// WithAuthenticated creates a new user which is authenticated (logged in)
func (t *AuthClientMock) NewAccount(uuid string) *Account {
	return &Account{
		mock: t,
		uuid: uuid,
	}
}

func (t *AuthClientMock) IsAuthenticated(ctx *jonson.Context) (*string, error) {
	existing, err := ctx.GetRequired(typeTestAccount)
	if err != nil {
		return nil, nil
	}
	uuid := existing.(*Account).uuid

	// check if account has been authenticated
	_, ok := t.authenticated[uuid]
	if !ok {
		return nil, nil
	}

	return &uuid, nil
}

func (t *AuthClientMock) IsAuthorized(ctx *jonson.Context) (*string, error) {
	existing, err := ctx.GetRequired(typeTestAccount)
	if err != nil {
		return nil, nil
	}
	uuid := existing.(*Account).uuid
	// first, let's check if the account has been authorized to call all methods
	_, ok := t.fullAccess[uuid]
	if ok {
		return &uuid, nil
	}

	meta := jonson.RequireRpcMeta(ctx)
	for _, v := range t.methodAccess[uuid] {
		if v.Method != meta.Method {
			continue
		}
		if v.HttpMethod != meta.HttpMethod {
			continue
		}
		return &uuid, nil // access granted
	}

	// no access granted
	return nil, nil
}
