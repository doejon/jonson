package jonson

import (
	"context"
	"testing"
)

func TestImpersonation(t *testing.T) {

	fac := NewFactory()
	impersonationProvider := NewImpersonatorProvider()
	fac.RegisterProvider(impersonationProvider)
	aliceUuid := "5362de3c-61fb-400c-9190-7b771403b07d"
	bobUuid := "5091ae7b-dba4-45d2-913a-e5a7f12b7bae"
	charlyUuid := "98a9dda0-1949-40dc-8c58-1378766d5992"

	assertAccountUuid := func(t *testing.T, ctx *Context, accountUuid string, accountUuids []string) {
		t.Helper()
		impersonated := RequireImpersonated(ctx)
		if impersonated.AccountUuid() != accountUuid {
			t.Fatalf("expected accountUuid to match, got (%s) vs (%s)", impersonated.AccountUuid(), accountUuid)
		}
		uuids := impersonated.TracedAccountUuids()
		if len(uuids) != len(accountUuids) {
			t.Fatalf("expected account uuids to match, got: (%v) vs (%v)", uuids, accountUuids)
		}
		for idx, v := range uuids {
			if accountUuids[idx] != v {
				t.Fatalf("expected account uuid in account uuids[%d] to match, got: (%s) vs (%s)", idx, v, accountUuids[idx])
			}
		}
	}

	t.Run("can get single impersonated account", func(t *testing.T) {
		ctx := NewContext(context.Background(), fac, nil)

		RequireImpersonator(ctx).Impersonate(aliceUuid, func(ctx *Context) error {
			assertAccountUuid(t, ctx, aliceUuid, []string{aliceUuid})
			return nil
		})
	})

	t.Run("can get multiple impersonated accounts when nesting impersonation", func(t *testing.T) {
		ctx := NewContext(context.Background(), fac, nil)

		RequireImpersonator(ctx).Impersonate(aliceUuid, func(ctx *Context) error {
			assertAccountUuid(t, ctx, aliceUuid, []string{aliceUuid})

			return RequireImpersonator(ctx).Impersonate(bobUuid, func(ctx *Context) error {
				assertAccountUuid(t, ctx, bobUuid, []string{aliceUuid, bobUuid})

				return RequireImpersonator(ctx).Impersonate(charlyUuid, func(ctx *Context) error {
					assertAccountUuid(t, ctx, charlyUuid, []string{aliceUuid, bobUuid, charlyUuid})
					return nil
				})
			})
		})
	})
}
