package account

import "github.com/doejon/jonson"

// @generate
type Private struct {
}

func (p *Private) AccountUuid() string {
	return "70634da0-7459-4a17-a50f-7afc2a600d50"
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
	r := jonson.RequireHttpRequest(ctx)
	authenticated := r.Header.Get("Authorization")
	if authenticated != "authorized" {
		panic("account is not authorized; please set the Authorization header to 'authorized'")
	}

	return &Private{}
}

func (p *AuthenticationProvider) NewPublic(ctx *jonson.Context) *Public {
	return &Public{}
}
