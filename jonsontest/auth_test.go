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

// System will be used to test API calls which are nested
type System struct {
}

func (s *System) GetV1(ctx *jonson.Context, caller *jonson.Private) error {
	return nil
}

func GetV1(ctx *jonson.Context) error {
	_, err := ctx.CallMethod("system/get.v1", jonson.RpcHttpMethodPost, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

// SetV1 calls GetV1
func (s *System) SetV1(ctx *jonson.Context, caller *jonson.Private) error {
	err := GetV1(ctx)
	if err != nil {
		return err
	}
	return nil
}

func SetV1(ctx *jonson.Context) error {
	_, err := ctx.CallMethod("system/set.v1", jonson.RpcHttpMethodPost, nil, nil)
	if err != nil {
		return err
	}

	return nil
}

func TestAuthNestedCalls(t *testing.T) {

	mock := NewAuthClientMock()
	fac := jonson.NewFactory()
	fac.RegisterProvider(jonson.NewAuthProvider(mock))

	mtd := jonson.NewMethodHandler(fac, jonson.NewDebugSecret(), nil)
	mtd.RegisterSystem(&System{})

	accSuperUser := mock.NewAccount("e6dd1e60-8969-4f08-a854-80a29b69d7f3").Authorized()
	accLimited := mock.NewAccount("efe59f3f-ed42-4534-8db1-3f7f7e94752e").Authorized(&RpcMethod{
		HttpMethod: jonson.RpcHttpMethodPost,
		Method:     "system/set.v1",
	})

	accLimited2 := mock.NewAccount("1ce976a0-9c0a-4969-b21a-c3c531ccbafe").Authorized(&RpcMethod{
		HttpMethod: jonson.RpcHttpMethodPost,
		Method:     "system/get.v1",
	})

	t.Run("accSuperUser can access set and get", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd, accSuperUser.Provide).MustRun(func(ctx *jonson.Context) error {
			return GetV1(ctx)
		})
		NewContextBoundary(t, fac, mtd, accSuperUser.Provide).MustRun(func(ctx *jonson.Context) error {
			return SetV1(ctx)
		})
	})

	t.Run("accLimited2 can call get but not set", func(t *testing.T) {
		NewContextBoundary(t, fac, mtd, accLimited2.Provide).MustRun(func(ctx *jonson.Context) error {
			return GetV1(ctx)
		})

		err := NewContextBoundary(t, fac, mtd, accLimited2.Provide).Run(func(ctx *jonson.Context) error {
			return SetV1(ctx)
		})
		if err == nil {
			t.Fatal("expected err != nil")
		}
		if err.(*jonson.Error).Code != jonson.ErrUnauthorized.Code {
			t.Fatalf("expected err to be unauthorized, got: %s", err)
		}
	})

	t.Run("accLimited can not access both since set also calls get", func(t *testing.T) {
		err := NewContextBoundary(t, fac, mtd, accLimited.Provide).Run(func(ctx *jonson.Context) error {
			return GetV1(ctx)
		})
		if err == nil {
			t.Fatal("expected err != nil")
		}
		if err.(*jonson.Error).Code != jonson.ErrUnauthorized.Code {
			t.Fatalf("expected err to be unauthorized, got: %s", err)
		}

		err = NewContextBoundary(t, fac, mtd, accLimited.Provide).Run(func(ctx *jonson.Context) error {
			return SetV1(ctx)
		})
		if err == nil {
			t.Fatal("expected err != nil")
		}
		if err.(*jonson.Error).Code != jonson.ErrUnauthorized.Code {
			t.Fatalf("expected err to be unauthorized, got: %s", err)
		}
	})

	t.Run("accLimited can access both get and set after authorization has been granted", func(t *testing.T) {
		accLimited.Authorized(&RpcMethod{
			HttpMethod: jonson.RpcHttpMethodPost,
			Method:     "system/get.v1",
		})
		NewContextBoundary(t, fac, mtd, accLimited.Provide).MustRun(func(ctx *jonson.Context) error {
			return GetV1(ctx)
		})

		NewContextBoundary(t, fac, mtd, accLimited.Provide).MustRun(func(ctx *jonson.Context) error {
			return SetV1(ctx)
		})

	})
}
