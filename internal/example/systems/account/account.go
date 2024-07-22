package account

import "github.com/doejon/jonson"

//go:generate go run github.com/doejon/jonson/cmd/generate

type Account struct{}

func NewAccount() *Account {
	return &Account{}
}

var ErrNotFound = &jonson.Error{Code: 10000, Message: "Account not found"}

type MeV1Result struct {
	Uuid string `json:"uuid"`
	Name string `json:"name"`
}

func (a *Account) MeV1(ctx *jonson.Context, caller *Private, _ jonson.HttpGet) (*MeV1Result, error) {
	return &MeV1Result{
		Uuid: caller.AccountUuid(),
		Name: "Silvio",
	}, nil
}

type GetProfileV1Params struct {
	jonson.Params
	Uuid string `json:"uuid"`
}

func (g *GetProfileV1Params) JonsonValidate(v *jonson.Validator) {
	if len(g.Uuid) != 36 {
		v.Path("uuid").Message("uuid invalid")
	}
}

type GetProfileV1Result struct {
	Name string `json:"name"`
}

func (a *Account) GetProfileV1(ctx *jonson.Context, caller *Public, _ jonson.HttpPost, params *GetProfileV1Params) (*GetProfileV1Result, error) {
	uuid := "70634da0-7459-4a17-a50f-7afc2a600d50"
	if params.Uuid != uuid {
		return nil, ErrNotFound
	}
	return &GetProfileV1Result{
		Name: "Silvio",
	}, nil
}
