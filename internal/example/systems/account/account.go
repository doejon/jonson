package account

import (
	"log"
	"time"

	"github.com/doejon/jonson"
)

//go:generate go run github.com/doejon/jonson/cmd/generate -jonson=github.com/doejon/jonson

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
	required := jonson.RequireLogger(ctx)
	required.Info("calling MeV1")
	a.subFnCall(ctx)

	return &MeV1Result{
		Uuid: caller.AccountUuid(),
		Name: "Silvio",
	}, nil
}

func (a *Account) subFnCall(ctx *jonson.Context) {
	jonson.RequireLogger(ctx).Info("calling subFnCall")
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
	jonson.RequireLogger(ctx).Info("calling GetProfileV1")

	uuid := "70634da0-7459-4a17-a50f-7afc2a600d50"
	if params.Uuid != uuid {
		return nil, ErrNotFound
	}
	return &GetProfileV1Result{
		Name: "Silvio",
	}, nil
}

func (a *Account) ProcessV1(ctx *jonson.Context, caller *Public, _ jonson.HttpGet) error {
	jonson.RequireLogger(ctx).Info("calling ProcessV1")

	graceful := jonson.RequireGraceful(ctx)
	for graceful.IsUp() {
		for i := 0; i < 5; i++ {
			log.Printf("sleeping %d", i+1)
			time.Sleep(time.Second * 1)
		}
	}
	jonson.RequireLogger(ctx).Info("exiting account/process.v1, server shutting down")
	return nil
}
