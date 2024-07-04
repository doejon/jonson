package account

import "github.com/doejon/jonson"

// @generate
type Private struct {
}

// @generate
type Public struct {
}

type AuthenticationProvider struct {
}

func NewAuthenticationProvider() *AuthenticationProvider {
	return &AuthenticationProvider{}
}

func (p *AuthenticationProvider) NewPrivate(ctx *jonson.Context) *Private {
	r := jonson.RequireHTTPRequest(ctx)
	authenticated := r.Header.Get("Authorization")
	if authenticated != "authorized" {
		panic("account is not authorized; please set the Authorization header to 'authorized'")
	}

	return &Private{}
}

func (p *AuthenticationProvider) NewPublic(ctx *jonson.Context) *Public {
	return &Public{}
}
