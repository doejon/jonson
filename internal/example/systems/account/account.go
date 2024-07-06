package account

import "github.com/doejon/jonson"

//go:generate go run github.com/doejon/jonson/cmd/generate

// @generate
type Account struct{}

func NewAccount() *Account {
	return &Account{}
}

type AccountV1Result struct {
	Uuid string `json:"uuid"`
	Name string `json:"name"`
}

type AccountV1Params struct {
	jonson.Params
	Uuid string `json:"uuid"`
}

var ErrNotFound = &jonson.Error{Code: 10000, Message: "Account not found"}

func (a *Account) GetV1(ctx *jonson.Context, caller *Private, params *AccountV1Params) (*AccountV1Result, error) {
	uuid := "70634da0-7459-4a17-a50f-7afc2a600d50"
	if params.Uuid != uuid {
		return nil, ErrNotFound
	}
	return &AccountV1Result{
		Uuid: uuid,
		Name: "Silvio",
	}, nil
}
