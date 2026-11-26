package account

import (
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
	// Uuid of the account
	Uuid string `json:"uuid"`
}

func (g *GetProfileV1Params) JonsonValidate(v *jonson.Validator) {
	v.Path("uuid").Check(len(g.Uuid) == 36, "uuid invalid")
}

type GetProfileV1Result struct {
	Name string `json:"name"`
}

// GetProfileV1 returns the name of the account with the given uuid.
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

type SubmitPreferencesV1Params struct {
	jonson.Params
	Name      string   `json:"name"`           // name of the account holder
	Age       int      `json:"age"`            // age in years
	Marketing bool     `json:"marketing"`      // whether the account accepts marketing messages
	Tags      []string `json:"tags,omitempty"` // free-form interest tags
}

func (p *SubmitPreferencesV1Params) JonsonValidate(v *jonson.Validator) {
	v.Path("name").Check(p.Name != "", "name is required")
	v.Path("age").Check(p.Age >= 0, "age must be zero or greater")
	v.Path("marketing").Skip()
	v.Path("tags").Skip()
}

type SubmitPreferencesV1Result struct {
	Summary string   `json:"summary"`
	Name    string   `json:"name"`
	Age     int      `json:"age"`
	Tags    []string `json:"tags"`
}

// SubmitPreferencesV1 stores the marketing preferences of an account and returns a summary.
func (a *Account) SubmitPreferencesV1(ctx *jonson.Context, params *SubmitPreferencesV1Params) (*SubmitPreferencesV1Result, error) {
	jonson.RequireLogger(ctx).Info("calling SubmitPreferencesV1")

	summary := "marketing disabled"
	if params.Marketing {
		summary = "marketing enabled"
	}

	return &SubmitPreferencesV1Result{
		Summary: summary,
		Name:    params.Name,
		Age:     params.Age,
		Tags:    params.Tags,
	}, nil
}

type QueryPreviewV1Params struct {
	jonson.Params
	Count   int      `json:"count"`
	Enabled bool     `json:"enabled"`
	Tags    []string `json:"tags"`
}

func (p *QueryPreviewV1Params) JonsonValidate(v *jonson.Validator) {
	v.Path("count").Check(p.Count > 0, "count must be greater than 0")
	v.Path("enabled").Skip()
	v.Path("tags").Check(len(p.Tags) > 0, "at least one tag is required")
}

type QueryPreviewV1Result struct {
	Count   int      `json:"count"`
	Enabled bool     `json:"enabled"`
	Tags    []string `json:"tags"`
	Summary string   `json:"summary"`
}

// QueryPreviewV1 returns the given count, enabled flag and tags together with a summary.
func (a *Account) QueryPreviewV1(ctx *jonson.Context, caller *Public, _ jonson.HttpPost, params *QueryPreviewV1Params) (*QueryPreviewV1Result, error) {
	jonson.RequireLogger(ctx).Info("calling QueryPreviewV1")

	summary := "disabled"
	if params.Enabled {
		summary = "enabled"
	}

	return &QueryPreviewV1Result{
		Count:   params.Count,
		Enabled: params.Enabled,
		Tags:    params.Tags,
		Summary: summary,
	}, nil
}

func (a *Account) ProcessV1(ctx *jonson.Context, caller *Public, _ jonson.HttpGet) error {
	jonson.RequireLogger(ctx).Info("calling ProcessV1")

	graceful := jonson.RequireGraceful(ctx)
	for graceful.IsUp() {
		select {
		case <-ctx.Done():
			jonson.RequireLogger(ctx).Info("exiting account/process.v1 - client disconnected")
			return nil
		case <-time.After(time.Second * 3):
			// check if isUp finished
		}

	}
	jonson.RequireLogger(ctx).Info("exiting account/process.v1, server shutting down")
	return nil
}
