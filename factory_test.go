package jonson

import (
	"context"
	"reflect"
	"testing"
	"time"
)

type mockTime struct {
	Shareable
	now time.Time
}

func newMockTime(now time.Time) *mockTime {
	return &mockTime{
		now: now,
	}
}

func (m *mockTime) Now() time.Time {
	return m.now
}

func (m *mockTime) Sleep(time.Duration) {
	// no-op
}

type TestProvider struct {
	loggedIn bool
}

func (t *TestProvider) setLoggedIn(canResolve bool) {
	t.loggedIn = canResolve
}

func NewTestProvider() *TestProvider {

	return &TestProvider{
		loggedIn: false,
	}
}

type TestPrivate struct {
}

func (p *TestPrivate) AccountUuid() string {
	return testAccountUuid
}

var TypeTestPrivate = reflect.TypeOf((**TestPrivate)(nil)).Elem()

// RequireHttpResponseWriter returns the current http response writer
// which is used to handle the ongoing request's response.
func RequireTestPrivate(ctx *Context) *TestPrivate {
	if v := ctx.Require(TypeTestPrivate); v != nil {
		return v.(*TestPrivate)
	}
	return nil
}

func (t *TestProvider) NewTestPrivate(ctx *Context) *TestPrivate {
	if !t.loggedIn {
		panic(ErrUnauthorized)
	}
	return &TestPrivate{}
}

type TestPublic struct {
}

var TypeTestPublic = reflect.TypeOf((**TestPublic)(nil)).Elem()

// RequireHttpResponseWriter returns the current http response writer
// which is used to handle the ongoing request's response.
func RequireTestPublic(ctx *Context) *TestPublic {
	if v := ctx.Require(TypeTestPublic); v != nil {
		return v.(*TestPublic)
	}
	return nil
}

func (t *TestProvider) NewTestPublic(ctx *Context) *TestPublic {
	return &TestPublic{}
}

func TestFactory(t *testing.T) {
	fac := NewFactory()
	enc := NewDebugSecret()
	methodHandler := NewMethodHandler(fac, enc, nil)

	t.Run("test registering provider func", func(t *testing.T) {
		tm := time.Now()
		var TypeTime = reflect.TypeOf((**time.Time)(nil)).Elem()
		provideTime := func(ctx *Context) *time.Time {
			return &tm
		}

		fac.RegisterProviderFunc(provideTime)

		ctx := NewContext(context.Background(), fac, methodHandler)
		tmProvided := fac.Provide(ctx, TypeTime)

		if tmProvided != &tm {
			t.Fatal("expected time provided to equal time from provider func")
		}
	})

}
