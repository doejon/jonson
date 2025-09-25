package jonson

import (
	"context"
	"fmt"
	"testing"
	"time"
)

type TestSystem struct {
}

func NewTestSystem() *TestSystem {
	return &TestSystem{}
}

type CurrentTimeV1Result struct {
	Ts int64 `json:"ts"`
}

func (t *TestSystem) CurrentTimeV1(ctx *Context, public *TestPublic, _ HttpGet) (*CurrentTimeV1Result, error) {
	nw := t.getCurrentTime(ctx)
	RequireLogger(ctx).Info("current time", "currentTime", nw)

	return &CurrentTimeV1Result{
		Ts: nw,
	}, nil
}

func (t *TestSystem) getCurrentTime(ctx *Context) int64 {
	nw := RequireTime(ctx).Now().Unix()
	RequireLogger(ctx).Info("getCurrentTime()", "currentTime", nw)
	return nw
}

type MeV1Result struct {
	Uuid       string
	Name       string
	HttpMethod RpcHttpMethod
}

func (t *TestSystem) MeV1(ctx *Context, private *TestPrivate, _ HttpGet) (*MeV1Result, error) {
	return &MeV1Result{
		Uuid:       private.AccountUuid(),
		Name:       "Silvio",
		HttpMethod: RequireRpcMeta(ctx).HttpMethod,
	}, nil
}

func (t *TestSystem) MeErrorV1(ctx *Context, private *TestPrivate, _ HttpGet) (*MeV1Result, error) {
	return nil, ErrInternal.CloneWithData(&ErrorData{
		Details: []*Error{
			{
				Code:    10000,
				Message: "failed to retrieve profiles",
			},
		},
	})
}

// CheckRequirables makes sure all default requireables are available
// which are available for all handlers (http, rpc over http, ws)
func (t *TestSystem) CheckRequirablesV1(ctx *Context, public *TestPublic, _ HttpGet) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = getRecoverError(r)
		}
	}()

	RequireHttpRequest(ctx)
	RequireHttpResponseWriter(ctx)
	RequireSecret(ctx)
	RequireRpcMeta(ctx)
	return nil
}

type GetProfileV1Params struct {
	Params
	Uuid string `json:"uuid"`
}

func (g *GetProfileV1Params) JonsonValidate(v *Validator) {
	if len(g.Uuid) > 36 {
		v.Path("uuid").Message("uuid invalid")
	}
}

type GetProfileV1Result struct {
	Name       string `json:"name"`
	HttpMethod RpcHttpMethod
}

func (t *TestSystem) GetProfileV1(ctx *Context, private *TestPrivate, _ HttpPost, params *GetProfileV1Params) (*GetProfileV1Result, error) {
	if params.Uuid != testAccountUuid {
		return nil, ErrInvalidParams.CloneWithData(&ErrorData{
			Details: []*Error{
				{
					Code:    10001,
					Message: "profile not found",
				},
			},
		})
	}
	return &GetProfileV1Result{
		Name:       "Silvio",
		HttpMethod: RequireRpcMeta(ctx).HttpMethod,
	}, nil
}

func TestMethodHandler(t *testing.T) {
	tm := time.Now()
	testProvider := NewTestProvider()

	factory := NewFactory()
	factory.RegisterProvider(testProvider)
	factory.RegisterProvider(NewTimeProvider(func() Time {
		return newMockTime(tm)
	}))

	testSystem := NewTestSystem()

	secret := NewDebugSecret()
	methodHandler := NewMethodHandler(factory, secret, &MethodHandlerOptions{
		MissingValidationLevel: MissingValidationLevelFatal,
	})
	methodHandler.RegisterSystem(testSystem)

	t.Run("can get current time", func(t *testing.T) {
		ctx := NewContext(context.Background(), factory, methodHandler)
		_res, err := methodHandler.CallMethod(ctx, "test-system/current-time.v1", RpcHttpMethodGet, nil)
		if err != nil {
			t.Fatal(err)
		}

		res := _res.(*CurrentTimeV1Result)
		if res.Ts != tm.Unix() {
			t.Fatalf("expected time to match, got: %d %d", res.Ts, tm.Unix())
		}
	})

	t.Run("returns error on errorness endpoint", func(t *testing.T) {
		// toggle flag to grant access to private
		testProvider.setLoggedIn(true)

		ctx := NewContext(context.Background(), factory, methodHandler)
		_, err := methodHandler.CallMethod(ctx, "test-system/me-error.v1", RpcHttpMethodGet, nil)
		if err == nil {
			t.Fatal("expected call to fail")
		}

		errRes := err.(*Error)
		if errRes.Code != ErrInternal.Code {
			t.Fatalf("expected err internal, got: %v", errRes.Code)
		}
	})

	t.Run("fails accessing 'me' due to missing authorization", func(t *testing.T) {
		// toggle flag to grant access to private
		testProvider.setLoggedIn(false)

		ctx := NewContext(context.Background(), factory, methodHandler)
		_, err := methodHandler.CallMethod(ctx, "test-system/me-error.v1", RpcHttpMethodGet, nil)
		if err == nil {
			t.Fatal("expected call to fail")
		}

		errRes := err.(*Error)
		if errRes.Code != ErrUnauthorized.Code {
			t.Fatalf("expected err unauthorized, got: %v", errRes.Code)
		}
	})

	t.Run("first can access and then won't be able to access due to missing permissions using the same initial context", func(t *testing.T) {
		// toggle flag to grant access to private
		testProvider.setLoggedIn(true)

		ctx := NewContext(context.Background(), factory, methodHandler)
		_, err := methodHandler.CallMethod(ctx, "test-system/me.v1", RpcHttpMethodGet, nil)
		if err != nil {
			t.Fatalf("expected call succeed: %s", err)
		}

		testProvider.setLoggedIn(false)
		_, err = methodHandler.CallMethod(ctx, "test-system/me.v1", RpcHttpMethodGet, nil)
		if err == nil {
			t.Fatalf("expected call fail: user got logged out")
		}
	})

	t.Run("http method is GET", func(t *testing.T) {
		// toggle flag to grant access to private
		testProvider.setLoggedIn(true)

		ctx := NewContext(context.Background(), factory, methodHandler)
		_res, err := methodHandler.CallMethod(ctx, "test-system/me.v1", RpcHttpMethodGet, nil)

		if err != nil {
			t.Fatalf("expected call succeed: %s", err)
		}
		res := _res.(*MeV1Result)

		if res.HttpMethod != RpcHttpMethodGet {
			t.Fatalf("expected http method to equal GET, got: %s", res.HttpMethod)
		}

	})

	t.Run("http method is POST", func(t *testing.T) {
		// toggle flag to grant access to private
		testProvider.setLoggedIn(true)

		ctx := NewContext(context.Background(), factory, methodHandler)
		_res, err := methodHandler.CallMethod(ctx, "test-system/get-profile.v1", RpcHttpMethodPost, &GetProfileV1Params{
			Uuid: testAccountUuid,
		})

		if err != nil {
			t.Fatalf("expected call succeed: %s", err)
		}
		res := _res.(*GetProfileV1Result)

		if res.HttpMethod != RpcHttpMethodPost {
			t.Fatalf("expected http method to equal POST, got: %s", res.HttpMethod)
		}

	})

}

func TestMethodNameHelpers(t *testing.T) {
	type methodTest struct {
		slug string

		version uint64
		method  string
		system  string
	}

	for _, v := range []methodTest{
		{
			slug:    "MyMethodV1",
			version: 1,
			method:  "my-method",
		},
		{
			slug:    "MyMethodx",
			version: 0,
			method:  "",
		},
	} {
		t.Run(fmt.Sprintf("expect method/version %s to result in method '%s' and version %d", v.slug, v.method, v.version), func(t *testing.T) {
			method, version := SplitGoMethodName(v.slug)
			if method != v.method {
				t.Fatal("expected method to be the same", v.method, method)
			}
			if version != v.version {
				t.Fatal("expected version to be the same", v.version, version)
			}
		})
	}

	for _, v := range []methodTest{
		{
			slug:    "sys1/my-method.v10",
			version: 10,
			method:  "my-method",
			system:  "sys1",
		},
		{
			slug:    "sys2/my-other-method.v1",
			version: 1,
			method:  "my-other-method",
			system:  "sys2",
		},
	} {
		t.Run(fmt.Sprintf("expect generated system (%s) method (%s) version (%d) to result in slug '%s'", v.system, v.method, v.version, v.slug), func(t *testing.T) {
			slug := GetDefaultMethodName(v.system, v.method, v.version)
			if slug != v.slug {
				t.Fatal("expected slug to match", slug, v.slug)
			}
		})
	}

	for _, v := range []methodTest{
		{
			slug:    "sys1/my-method.v10",
			version: 10,
			method:  "my-method",
			system:  "sys1",
		},
		{
			slug:    "sys2/my-other-method.v1",
			version: 1,
			method:  "my-other-method",
			system:  "sys2",
		},
		{
			slug:    "Sys2/my-other-method.v1",
			version: 0,
			method:  "",
			system:  "",
		},
		{
			slug:    "sys/My-other-method.v1",
			version: 0,
			method:  "",
			system:  "",
		},
		{
			slug:    "sys/my-other-method.vx",
			version: 0,
			method:  "",
			system:  "",
		},
		{
			slug:    "sys/my-other-method",
			version: 0,
			method:  "",
			system:  "",
		},
		{
			slug:    "sys/my-other-method*.v1",
			version: 0,
			method:  "",
			system:  "",
		},
		{
			slug:    "sys!/my-other-method.v1",
			version: 0,
			method:  "",
			system:  "",
		},
		{
			slug:    "sys/my-other-method.v-1",
			version: 0,
			method:  "",
			system:  "",
		},
	} {
		t.Run(fmt.Sprintf("expect slug (%s) to result in system (%s) method (%s) version (%d)", v.slug, v.system, v.method, v.version), func(t *testing.T) {
			sys, method, version, _ := ParseRpcMethod(v.slug)
			if sys != v.system {
				t.Fatal("expected system to match", sys, v.system)
			}
			if method != v.method {
				t.Fatal("expected method to match", method, v.method)
			}
			if version != v.version {
				t.Fatal("expected version to match", version, v.version)
			}
		})
	}
}
