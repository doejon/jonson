package jonsontest

import (
	"fmt"
	"testing"

	"github.com/doejon/jonson"
)

func TestAuth(t *testing.T) {
	fac := jonson.NewFactory()

	mtd := jonson.NewMethodHandler(fac, jonson.NewDebugSecret(), nil)

	mock := NewAuthClientMock()
	fac.RegisterProvider(jonson.NewAuthProvider(mock))

	notAuthenticatedAccount := mock.NewAccount("e6dd1e60-8969-4f08-a854-80a29b69d7f3")
	authenticatedAccount := mock.NewAccount("bc570cf4-2eda-4047-83dd-f40c420b0189").Authenticated()
	authorizedAccount := mock.NewAccount("99ec08bd-f91d-4d20-8eb6-3eb05ee5a809").Authorized()

	meta := &jonson.RpcMeta{
		Method:     "/account/test.v1",
		HttpMethod: "GET",
	}

	authorizedMethodAccount := mock.NewAccount("a9e1071f-eac8-49a2-ad63-f242aa6d9b6b").Authorized(&RpcMethod{
		Method:     meta.Method,
		HttpMethod: meta.HttpMethod,
	})

	t.Run("no account provided, no uuid", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd).MustRun(func(ctx *jonson.Context) error {
			uuid, _ := jonson.RequirePublic(ctx).AccountUuid(ctx)
			if uuid != nil {
				return fmt.Errorf("expected uuid to be nil, got: %s", *uuid)
			}
			return nil
		})
	})

	t.Run("account provided but not authenticated", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd, notAuthenticatedAccount.Provide).MustRun(func(ctx *jonson.Context) error {
			uuid, _ := jonson.RequirePublic(ctx).AccountUuid(ctx)
			if uuid != nil {
				return fmt.Errorf("expected uuid to be nil, got: %s", *uuid)
			}
			return nil
		})
	})

	t.Run("account provided, authenticated but not authorized", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd, authenticatedAccount.Provide).MustRun(func(ctx *jonson.Context) error {
			uuid, _ := jonson.RequirePublic(ctx).AccountUuid(ctx)
			if uuid == nil {
				return fmt.Errorf("expected uuid, got nil")
			}
			if *uuid != authenticatedAccount.uuid {
				return fmt.Errorf("expected uuids to match, got: %s | %s", *uuid, authenticatedAccount.uuid)
			}
			return nil
		})

		err := NewContextBoundary(t, fac, mtd,
			func(ctx *jonson.Context) {
				ctx.StoreValue(jonson.TypeRpcMeta, meta)
			},
			authenticatedAccount.Provide,
		).Run(func(ctx *jonson.Context) error {
			jonson.RequirePrivate(ctx)
			return nil
		})
		if err == nil {
			t.Fatal("expected private to panic, no auth")
		}
		if err.(*jonson.Error).Code != jonson.ErrUnauthorized.Code {
			t.Fatalf("expected error to match unauthorized, got: %s", err)
		}
	})

	t.Run("account provided, authorized to all routes", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd, authorizedAccount.Provide).MustRun(func(ctx *jonson.Context) error {
			uuid, _ := jonson.RequirePublic(ctx).AccountUuid(ctx)
			if uuid == nil {
				return fmt.Errorf("expected uuid, got nil")
			}
			if *uuid != authorizedAccount.uuid {
				return fmt.Errorf("expected uuids to match, got: %s | %s", *uuid, authorizedAccount.uuid)
			}
			return nil
		})

		NewContextBoundary(t, fac, mtd,
			func(ctx *jonson.Context) {
				ctx.StoreValue(jonson.TypeRpcMeta, meta)
			},
			authorizedAccount.Provide,
		).MustRun(func(ctx *jonson.Context) error {
			uuid := jonson.RequirePrivate(ctx).AccountUuid()
			if uuid != authorizedAccount.uuid {
				return fmt.Errorf("expected uuids to match, got: %s | %s", uuid, authorizedAccount.uuid)
			}
			return nil
		})
	})

	t.Run("account provided, authorized to a specific route", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd, authorizedMethodAccount.Provide).MustRun(func(ctx *jonson.Context) error {
			uuid, _ := jonson.RequirePublic(ctx).AccountUuid(ctx)
			if uuid == nil {
				return fmt.Errorf("expected uuid, got nil")
			}
			if *uuid != authorizedMethodAccount.uuid {
				return fmt.Errorf("expected uuids to match, got: %s | %s", *uuid, authorizedMethodAccount.uuid)
			}
			return nil
		})

		NewContextBoundary(t, fac, mtd,
			func(ctx *jonson.Context) {
				ctx.StoreValue(jonson.TypeRpcMeta, meta)
			},
			authorizedMethodAccount.Provide,
		).MustRun(func(ctx *jonson.Context) error {
			uuid := jonson.RequirePrivate(ctx).AccountUuid()
			if uuid != authorizedMethodAccount.uuid {
				return fmt.Errorf("expected uuids to match, got: %s | %s", uuid, authorizedMethodAccount.uuid)
			}
			return nil
		})

		err := NewContextBoundary(t, fac, mtd,
			func(ctx *jonson.Context) {
				ctx.StoreValue(jonson.TypeRpcMeta, &jonson.RpcMeta{
					Method:     meta.Method,
					HttpMethod: "POST",
				})
			},
			authorizedMethodAccount.Provide,
		).Run(func(ctx *jonson.Context) error {
			jonson.RequirePrivate(ctx)
			return nil
		})
		if err == nil {
			t.Fatalf("expected error to be nil, got: %s", err)
		}
		if err.(*jonson.Error).Code != jonson.ErrUnauthorized.Code {
			t.Fatalf("expected err unauthorized, got: %s", err)
		}

	})

}
