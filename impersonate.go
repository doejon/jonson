package jonson

import "reflect"

// ImpersonatorProvider allows us to provide
// impersonation functionality to jonson.
// Impersonation can be used to make calls towards the API on behalf of
// another user.
type ImpersonatorProvider struct {
}

func NewImpersonatorProvider() *ImpersonatorProvider {
	return &ImpersonatorProvider{}
}

// NewImpersonator instantiates a new impersonator instance
func (i *ImpersonatorProvider) NewImpersonator(ctx *Context) *Impersonator {
	return &Impersonator{
		// keep the main context here (outer scope)
		ctx: ctx,
	}
}

type Impersonator struct {
	ctx *Context
}

var TypeImpersonator = reflect.TypeOf((**Impersonator)(nil)).Elem()

// RequireImpersonator returns the impersonator
func RequireImpersonator(ctx *Context) *Impersonator {
	if v := ctx.Require(TypeImpersonator); v != nil {
		return v.(*Impersonator)
	}
	return nil
}

// Impersonated will be stored once an account has been impersonated.
// Use RequireImpersonated to gain access to the impersonated account.
type Impersonated struct {
	// The value needs to be shared
	// across calls done within a scope of an impersonation
	Shareable

	// The account uuid of the current impersonation
	accountUuid string

	// in case Alice impersonates Bob impersonates Charly,
	// we will see Bob and Charly's impersonation here.
	// the last account uuid in the slice equals accountUuid
	accountUuids []string
}

var TypeImpersonated = reflect.TypeOf((**Impersonated)(nil)).Elem()

// RequireImpersonated returns the current impersonated setting;
// In case no impersonation has been set, RequireImpersonated will panic
func RequireImpersonated(ctx *Context) *Impersonated {
	if v := ctx.Require(TypeImpersonated); v != nil {
		return v.(*Impersonated)
	}
	return nil
}

// RequireOptionalImpersonated returns impersonated in case it exists;
// In case no impersonation is taking place, RequireOptionalImpersonated
// will return nil
func RequireOptionalImpersonated(ctx *Context) *Impersonated {
	v, _ := ctx.GetValue(TypeImpersonated)
	if v == nil {
		return nil
	}
	return v.(*Impersonated)
}

func (i *Impersonated) AccountUuid() string {
	return i.accountUuid
}

// TracedAccountUuids returns _all_ impersonated account uuids
// of the current scope:
// in case Account Alice impersonates Account Bob who impersonates Account Charly,
// TracedAccountUuids() will return [BobUuid, CharlyUuid]
func (i *Impersonated) TracedAccountUuids() []string {
	// create a copy to protect the impersonated array from manipulation
	cpy := append([]string{}, i.accountUuids...)
	return cpy
}

// newImpersonated returns a new impersonated instance
func newImpersonated(existing *Impersonated, accountUuid string) *Impersonated {
	out := &Impersonated{
		accountUuid:  accountUuid,
		accountUuids: []string{},
	}
	if existing != nil {
		out.accountUuids = append(out.accountUuids, existing.accountUuids...)
	}
	out.accountUuids = append(out.accountUuids, out.accountUuid)

	return out
}

// Impersonate will impersonate an account.
// Once the impersonation happened, a new context will be created
// and the context will be in the scope of the impersonated account.
func (i *Impersonator) Impersonate(accountUuid string, fn func(ctx *Context) error) error {

	// we create a completely blank context
	// and will copy only those values
	// that are explicitly marked as shareable across
	// impersonations. Everything else will be ignored
	newContext := i.ctx.Fork()
	var existingImpersonation *Impersonated
	for _, v := range i.ctx.values {
		if !v.valid {
			continue
		}
		// we only keep those values that have
		// been marked explicitly shareable across impersonation
		if _, ok := v.val.(ShareableAcrossImpersonation); ok {
			newContext.StoreValue(v.rt, v.val)
		}
		if v.rt == TypeImpersonated {
			existingImpersonation = v.val.(*Impersonated)
		}
	}
	imp := newImpersonated(existingImpersonation, accountUuid)
	newContext.StoreValue(TypeImpersonated, imp)

	return fn(newContext)
}
