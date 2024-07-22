package account

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/doejon/jonson"
	"github.com/doejon/jonson/jonsontest"
)

var testUuid = "70634da0-7459-4a17-a50f-7afc2a600d50"

func TestAccount(t *testing.T) {
	factory := jonson.NewFactory()
	factory.RegisterProvider(NewAuthenticationProvider())

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
}
