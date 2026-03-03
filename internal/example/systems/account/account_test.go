package account

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/doejon/jonson"
	"github.com/doejon/jonson/jonsontest"
)

var testUuid = "70634da0-7459-4a17-a50f-7afc2a600d50"

func TestAccount(t *testing.T) {
	factory := jonson.NewFactory()
	factory.RegisterProvider(NewAuthenticationProvider())
	gracefulProvider := jonsontest.NewGracefulProvider()
	factory.RegisterProvider(gracefulProvider)

	secret := jonson.NewDebugSecret()

	methodHandler := jonson.NewMethodHandler(factory, secret, nil)
	methodHandler.RegisterSystem(NewAccount())

	t.Run("gets profile", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)

		var p *GetProfileV1Result
		contextBoundary.MustRun(func(ctx *jonson.Context) (err error) {
			p, err = GetProfileV1(ctx, &GetProfileV1Params{
				Uuid: testUuid,
			})
			return err
		})
		if p.Name != "Silvio" {
			t.Fatalf("expected name to equal Silvio, got: %s", p.Name)
		}
	})

	t.Run("gets profile fail with not found", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)

		err := contextBoundary.Run(func(ctx *jonson.Context) (err error) {
			_, err = GetProfileV1(ctx, &GetProfileV1Params{
				Uuid: "9b641812-b78c-40f0-a8df-88f8378f10a7",
			})
			return err
		})
		if err == nil {
			t.Fatal("expected error not not be nil due to unknown profile")
		}

		casted, ok := err.(*jonson.Error)
		if !ok {
			t.Fatalf("expected err not found. got: %s", err)
		}
		if casted.Code != ErrNotFound.Code {
			t.Fatalf("expected err not found. got: %s", err)
		}
	})

	t.Run("tries to get 'me' but fails due to missing authorization", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)

		err := contextBoundary.Run(func(ctx *jonson.Context) (err error) {
			_, err = MeV1(ctx)
			return err
		})
		if err == nil {
			t.Fatal("expected to have error, permission denied")
		}
	})

	t.Run("tries to get 'me' and succeeds due to authorization", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)

		req, _ := http.NewRequest("POST", "https://example.com", nil)
		req.Header.Set("Authorization", "authorized")

		var me *MeV1Result
		contextBoundary.WithHttpSource(req, httptest.NewRecorder()).MustRun(func(ctx *jonson.Context) (err error) {
			me, err = MeV1(ctx)
			return err
		})
		if me.Name != "Silvio" {
			t.Fatalf("expected name to equal Silvio, got: %s", me.Name)
		}

		if me.Uuid != testUuid {
			t.Fatalf("expected test uuid to equal me.Uuid, got: %s", me.Uuid)
		}
	})

	t.Run("calls generated submit-preferences wrapper", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)

		var result *SubmitPreferencesV1Result
		contextBoundary.MustRun(func(ctx *jonson.Context) (err error) {
			result, err = SubmitPreferencesV1(ctx, &SubmitPreferencesV1Params{
				Name:      "Silvio",
				Age:       34,
				Marketing: true,
				Tags:      []string{"dev", "go"},
			})
			return err
		})

		if result == nil {
			t.Fatal("expected result not to be nil")
		}
		if result.Summary != "marketing enabled" {
			t.Fatalf("expected summary to equal marketing enabled, got: %s", result.Summary)
		}
		if result.Name != "Silvio" {
			t.Fatalf("expected name to equal Silvio, got: %s", result.Name)
		}
		if result.Age != 34 {
			t.Fatalf("expected age to equal 34, got: %d", result.Age)
		}
		if len(result.Tags) != 2 || result.Tags[0] != "dev" || result.Tags[1] != "go" {
			t.Fatalf("expected tags to equal [dev go], got: %#v", result.Tags)
		}
	})

	t.Run("calls generated query-preview wrapper", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)

		var result *QueryPreviewV1Result
		contextBoundary.MustRun(func(ctx *jonson.Context) (err error) {
			result, err = QueryPreviewV1(ctx, &QueryPreviewV1Params{
				Count:   42,
				Enabled: true,
				Tags:    []string{"dev", "go"},
			})
			return err
		})

		if result == nil {
			t.Fatal("expected result not to be nil")
		}
		if result.Count != 42 {
			t.Fatalf("expected count to equal 42, got: %d", result.Count)
		}
		if !result.Enabled {
			t.Fatal("expected enabled to equal true")
		}
		if result.Summary != "enabled" {
			t.Fatalf("expected summary to equal enabled, got: %s", result.Summary)
		}
		if len(result.Tags) != 2 || result.Tags[0] != "dev" || result.Tags[1] != "go" {
			t.Fatalf("expected tags to equal [dev go], got: %#v", result.Tags)
		}
	})

	t.Run("query-preview returns invalid params when json body and query are both provided", func(t *testing.T) {
		httpHandler := jonson.NewHttpMethodHandler(methodHandler)
		wtr := httptest.NewRecorder()
		req, _ := http.NewRequest(
			"POST",
			"/account/query-preview.v1?count=42&enabled=true",
			strings.NewReader(`{"count":42,"enabled":true,"tags":["dev","go"]}`),
		)
		req.Header.Set("Content-Type", "application/json")

		if found := httpHandler.Handle(wtr, req); !found {
			t.Fatal("expected http handler to find account/query-preview.v1")
		}
		if wtr.Code != http.StatusBadRequest {
			t.Fatalf("expected status to equal %d, got: %d", http.StatusBadRequest, wtr.Code)
		}

		rpcErr := &jonson.Error{}
		if err := json.Unmarshal(wtr.Body.Bytes(), rpcErr); err != nil {
			t.Fatalf("failed to decode error response: %v", err)
		}
		if rpcErr.Code != jonson.ErrInvalidParams.Code {
			t.Fatalf("expected invalid params error code, got: %d", rpcErr.Code)
		}
	})

	t.Run("calls generated process wrapper", func(t *testing.T) {
		contextBoundary := jonsontest.NewContextBoundary(t, factory, methodHandler)
		gracefulProvider.Shutdown()
		defer gracefulProvider.Restart()

		err := contextBoundary.Run(func(ctx *jonson.Context) error {
			return ProcessV1(ctx)
		})
		if err != nil {
			t.Fatalf("expected process wrapper call to succeed, got: %v", err)
		}
	})
}
