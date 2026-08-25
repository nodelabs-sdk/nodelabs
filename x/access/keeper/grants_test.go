package keeper_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/collections"
	"cosmossdk.io/log/v2"
	"github.com/cosmos/cosmos-sdk/store/v2"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/access/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

const (
	scopeA = "scope-a"
	scopeB = "scope-b"
)

var grantsPrefix = collections.NewPrefix(1)

// newGrants builds a grant store over a fresh in-memory KVStore. It stands in
// for the consuming module's keeper, which owns the store this is a prefix of.
func newGrants(t testing.TB, module string, spec types.Spec) (keeper.Grants, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey("accesstest")

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger())
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	sb := collections.NewSchemaBuilder(runtime.NewKVStoreService(storeKey))
	g := keeper.NewGrants(sb, grantsPrefix, "grants", module, spec)
	_, err := sb.Build()
	require.NoError(t, err)

	ctx := sdk.NewContext(stateStore, tmproto.Header{}, false, log.NewNopLogger())

	return g, ctx
}

// scopedSpec mirrors x/license: actions scoped to a fixed set of resources,
// with one action declared module-wide.
func scopedSpec() types.Spec {
	return types.Spec{
		Actions: []string{"issue", "revoke", "type.create"},
		ScopeExists: func(_ context.Context, scope string) (bool, error) {
			return scope == scopeA || scope == scopeB, nil
		},
		Unscoped: []string{"type.create"},
	}
}

// openSpec mirrors x/network: no scope validator, so every grant is
// module-wide.
func openSpec() types.Spec {
	return types.Spec{Actions: []string{"operate", "configure"}}
}

func newScoped(t testing.TB) (keeper.Grants, sdk.Context) {
	t.Helper()
	return newGrants(t, "scopedmod", scopedSpec())
}

func newOpen(t testing.TB) (keeper.Grants, sdk.Context) {
	t.Helper()
	return newGrants(t, "openmod", openSpec())
}

// ---------------------------------------------------------------------------
// Construction
// ---------------------------------------------------------------------------

func TestNewGrantsPanicsOnInvalidSpec(t *testing.T) {
	require.PanicsWithError(t,
		`access spec for module "bad": all 2 actions are declared unscoped: drop ScopeExists instead`,
		func() {
			newGrants(t, "bad", types.Spec{
				Actions:     []string{"issue", "type.create"},
				ScopeExists: func(context.Context, string) (bool, error) { return true, nil },
				Unscoped:    []string{"issue", "type.create"},
			})
		})
}

func TestGrantsAccessors(t *testing.T) {
	g, _ := newScoped(t)

	require.Equal(t, "scopedmod", g.Module())
	require.Equal(t, []string{"issue", "revoke", "type.create"}, g.Spec().SortedActions())
}

// ---------------------------------------------------------------------------
// GrantTo
// ---------------------------------------------------------------------------

func TestGrantToScoped(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA, scopeB}},
		{Action: "revoke", Scopes: []string{scopeA}},
	}))

	for _, tc := range []struct {
		action, scope string
		want          bool
	}{
		{"issue", scopeA, true},
		{"issue", scopeB, true},
		{"revoke", scopeA, true},
		{"revoke", scopeB, false},
		{"issue", "", false},
	} {
		has, err := g.HasGrant(ctx, grantee, tc.action, tc.scope)
		require.NoError(t, err)
		require.Equal(t, tc.want, has, "%s/%s", tc.action, tc.scope)
		require.Equal(t, tc.want, g.Can(ctx, grantee, tc.action, tc.scope))
	}
}

func TestGrantToIsAUnion(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA}},
	}))
	// A second grant must not disturb the first, and re-granting an existing
	// pair is an idempotent overwrite.
	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA, scopeB}},
		{Action: "revoke", Scopes: []string{scopeB}},
	}))

	grants, err := g.Export(ctx)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{
		{Grantee: grantee, Action: "issue", Scope: scopeA},
		{Grantee: grantee, Action: "issue", Scope: scopeB},
		{Grantee: grantee, Action: "revoke", Scope: scopeB},
	}, grants)
}

func TestGrantToModuleWideModule(t *testing.T) {
	g, ctx := newOpen(t)
	grantee := sample.AccAddress()

	// A module with no scope validator takes grants with no scopes...
	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{{Action: "operate"}}))
	has, err := g.HasGrant(ctx, grantee, "operate", "")
	require.NoError(t, err)
	require.True(t, has)

	// ...and treats a supplied scope as an unconstrained opaque string.
	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "configure", Scopes: []string{"anything"}},
	}))
	has, err = g.HasGrant(ctx, grantee, "configure", "anything")
	require.NoError(t, err)
	require.True(t, has)
}

func TestGrantToUnscopedActionRejectsScope(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	// Rejected even though scopeA is a real resource: a module-wide action has
	// exactly one key form, so it must carry the empty scope.
	err := g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "type.create", Scopes: []string{scopeA}},
	})
	require.ErrorIs(t, err, types.ErrInvalidScope)
	require.ErrorContains(t, err, "is module-wide")
}

func TestGrantToScopedActionRequiresScope(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	err := g.GrantTo(ctx, grantee, []types.ActionScopes{{Action: "issue"}})
	require.ErrorIs(t, err, types.ErrInvalidScope)
	require.ErrorContains(t, err, "must not be empty")
}

func TestGrantToRejectsUnknownAction(t *testing.T) {
	g, ctx := newScoped(t)

	err := g.GrantTo(ctx, sample.AccAddress(), []types.ActionScopes{
		{Action: "transfer", Scopes: []string{scopeA}},
	})
	require.ErrorIs(t, err, types.ErrInvalidAction)
	require.ErrorContains(t, err, `module "scopedmod"`)
}

func TestGrantToRejectsUnknownScope(t *testing.T) {
	g, ctx := newScoped(t)

	err := g.GrantTo(ctx, sample.AccAddress(), []types.ActionScopes{
		{Action: "issue", Scopes: []string{"nope"}},
	})
	require.ErrorIs(t, err, types.ErrInvalidScope)
	require.ErrorContains(t, err, "does not exist")
}

func TestGrantToIsAtomic(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	// The valid entry precedes the invalid one; nothing may be written.
	err := g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA}},
		{Action: "revoke", Scopes: []string{"nope"}},
	})
	require.ErrorIs(t, err, types.ErrInvalidScope)
	require.ErrorContains(t, err, "grant 1:")

	grants, err := g.Export(ctx)
	require.NoError(t, err)
	require.Empty(t, grants)
}

func TestGrantToMixedGranularityTakesTwoCalls(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA}},
	}))
	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "type.create"},
	}))

	grants, err := g.Export(ctx)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{
		{Grantee: grantee, Action: "issue", Scope: scopeA},
		{Grantee: grantee, Action: "type.create", Scope: ""},
	}, grants)
}

func TestGrantToRejectsOversizedInput(t *testing.T) {
	g, ctx := newOpen(t)
	grantee := sample.AccAddress()

	tooMany := make([]types.ActionScopes, types.MaxGrants+1)
	for i := range tooMany {
		tooMany[i] = types.ActionScopes{Action: "operate"}
	}
	require.ErrorContains(t, g.GrantTo(ctx, grantee, tooMany), "exceeds max")

	tooManyScopes := make([]string, types.MaxGrants+1)
	for i := range tooManyScopes {
		tooManyScopes[i] = "s"
	}
	require.ErrorContains(t,
		g.GrantTo(ctx, grantee, []types.ActionScopes{{Action: "operate", Scopes: tooManyScopes}}),
		"scopes length")
}

func TestGrantToEmitsEvent(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA, scopeB}},
	}))

	events := ctx.EventManager().Events()
	require.Len(t, events, 1)
	require.Equal(t, types.EventTypeGrantAccess, events[0].Type)
}

func TestGrantToEmitsNoEventOnFailure(t *testing.T) {
	g, ctx := newScoped(t)

	require.Error(t, g.GrantTo(ctx, sample.AccAddress(), []types.ActionScopes{
		{Action: "issue", Scopes: []string{"nope"}},
	}))
	require.Empty(t, ctx.EventManager().Events())
}

// ---------------------------------------------------------------------------
// RevokeFrom
// ---------------------------------------------------------------------------

func TestRevokeFrom(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{scopeA, scopeB}},
	}))

	require.NoError(t, g.RevokeFrom(ctx, grantee, []types.ActionScope{
		{Action: "issue", Scope: scopeA},
	}))

	has, err := g.HasGrant(ctx, grantee, "issue", scopeA)
	require.NoError(t, err)
	require.False(t, has)

	has, err = g.HasGrant(ctx, grantee, "issue", scopeB)
	require.NoError(t, err)
	require.True(t, has, "revoking one scope must not touch the others")
}

func TestRevokeFromModuleWideAction(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	require.NoError(t, g.GrantTo(ctx, grantee, []types.ActionScopes{{Action: "type.create"}}))
	require.NoError(t, g.RevokeFrom(ctx, grantee, []types.ActionScope{{Action: "type.create"}}))

	has, err := g.HasGrant(ctx, grantee, "type.create", "")
	require.NoError(t, err)
	require.False(t, has)
}

func TestRevokeFromIsIdempotent(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	// Never granted, and an action outside the vocabulary: both are no-ops
	// rather than errors, so a revoke can always be safely re-sent.
	require.NoError(t, g.RevokeFrom(ctx, grantee, []types.ActionScope{
		{Action: "issue", Scope: scopeA},
		{Action: "retired.action", Scope: scopeA},
	}))
	require.NoError(t, g.RevokeFrom(ctx, grantee, []types.ActionScope{
		{Action: "issue", Scope: scopeA},
	}))
}

func TestRevokeFromRejectsOversizedInput(t *testing.T) {
	g, ctx := newOpen(t)

	tooMany := make([]types.ActionScope, types.MaxGrants+1)
	for i := range tooMany {
		tooMany[i] = types.ActionScope{Action: "operate"}
	}
	require.ErrorContains(t, g.RevokeFrom(ctx, sample.AccAddress(), tooMany), "exceeds max")
}

func TestRevokeFromEmitsEvent(t *testing.T) {
	g, ctx := newOpen(t)

	require.NoError(t, g.RevokeFrom(ctx, sample.AccAddress(), []types.ActionScope{{Action: "operate"}}))

	events := ctx.EventManager().Events()
	require.Len(t, events, 1)
	require.Equal(t, types.EventTypeRevokeAccess, events[0].Type)
}

// ---------------------------------------------------------------------------
// Raw accessors
// ---------------------------------------------------------------------------

func TestSetAndRemoveBypassTheSpec(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	// Set is the fixture/migration escape hatch: no vocabulary check, no
	// scope check, no event.
	require.NoError(t, g.Set(ctx, grantee, "retired.action", "nonexistent"))
	has, err := g.HasGrant(ctx, grantee, "retired.action", "nonexistent")
	require.NoError(t, err)
	require.True(t, has)
	require.Empty(t, ctx.EventManager().Events())

	require.NoError(t, g.Remove(ctx, grantee, "retired.action", "nonexistent"))
	has, err = g.HasGrant(ctx, grantee, "retired.action", "nonexistent")
	require.NoError(t, err)
	require.False(t, has)
}

// ---------------------------------------------------------------------------
// Genesis
// ---------------------------------------------------------------------------

func TestInitExportRoundTrip(t *testing.T) {
	g, ctx := newScoped(t)
	a, b := sample.AccAddress(), sample.AccAddress()

	in := []types.Grant{
		{Grantee: a, Action: "issue", Scope: scopeA},
		{Grantee: a, Action: "type.create"},
		{Grantee: b, Action: "revoke", Scope: scopeB},
	}
	require.NoError(t, g.Init(ctx, in))

	out, err := g.Export(ctx)
	require.NoError(t, err)
	require.ElementsMatch(t, in, out)
}

func TestExportIsInKeyOrder(t *testing.T) {
	g, ctx := newScoped(t)
	grantee := sample.AccAddress()

	// Written out of order; the export must come back sorted by key.
	require.NoError(t, g.Init(ctx, []types.Grant{
		{Grantee: grantee, Action: "revoke", Scope: scopeB},
		{Grantee: grantee, Action: "issue", Scope: scopeB},
		{Grantee: grantee, Action: "issue", Scope: scopeA},
	}))

	out, err := g.Export(ctx)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{
		{Grantee: grantee, Action: "issue", Scope: scopeA},
		{Grantee: grantee, Action: "issue", Scope: scopeB},
		{Grantee: grantee, Action: "revoke", Scope: scopeB},
	}, out)
}

func TestExportEmpty(t *testing.T) {
	g, ctx := newScoped(t)

	out, err := g.Export(ctx)
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestInitEnforcesTheSpec(t *testing.T) {
	tests := []struct {
		name    string
		grant   types.Grant
		wantErr error
	}{
		{
			name:    "unknown action",
			grant:   types.Grant{Action: "transfer", Scope: scopeA},
			wantErr: types.ErrInvalidAction,
		},
		{
			name:    "unknown scope",
			grant:   types.Grant{Action: "issue", Scope: "nope"},
			wantErr: types.ErrInvalidScope,
		},
		{
			name:    "scoped action with empty scope",
			grant:   types.Grant{Action: "issue"},
			wantErr: types.ErrInvalidScope,
		},
		{
			name:    "module-wide action with a scope",
			grant:   types.Grant{Action: "type.create", Scope: scopeA},
			wantErr: types.ErrInvalidScope,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, ctx := newScoped(t)
			tc.grant.Grantee = sample.AccAddress()

			err := g.Init(ctx, []types.Grant{tc.grant})
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestInitEmitsNoEvents(t *testing.T) {
	g, ctx := newScoped(t)

	require.NoError(t, g.Init(ctx, []types.Grant{
		{Grantee: sample.AccAddress(), Action: "issue", Scope: scopeA},
	}))
	require.Empty(t, ctx.EventManager().Events())
}
