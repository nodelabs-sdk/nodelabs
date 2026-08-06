package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/access/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// seeded returns a scoped store holding a fixed spread of grants across two
// grantees, plus those two addresses.
func seeded(t testing.TB) (keeper.Grants, sdk.Context, string, string) {
	t.Helper()

	g, ctx := newScoped(t)
	a, b := sample.AccAddress(), sample.AccAddress()

	require.NoError(t, g.Init(ctx, []types.Grant{
		{Grantee: a, Action: "issue", Scope: scopeA},
		{Grantee: a, Action: "issue", Scope: scopeB},
		{Grantee: a, Action: "type.create"},
		{Grantee: b, Action: "revoke", Scope: scopeA},
	}))

	return g, ctx, a, b
}

func TestPaginateAll(t *testing.T) {
	g, ctx, a, b := seeded(t)

	got, page, err := keeper.PaginateAll(ctx, g, nil)
	require.NoError(t, err)
	require.NotNil(t, page)
	require.ElementsMatch(t, []types.Grant{
		{Grantee: a, Action: "issue", Scope: scopeA},
		{Grantee: a, Action: "issue", Scope: scopeB},
		{Grantee: a, Action: "type.create"},
		{Grantee: b, Action: "revoke", Scope: scopeA},
	}, got)
}

func TestPaginateAllRespectsLimit(t *testing.T) {
	g, ctx, _, _ := seeded(t)

	first, page, err := keeper.PaginateAll(ctx, g, &query.PageRequest{Limit: 2})
	require.NoError(t, err)
	require.Len(t, first, 2)
	require.NotEmpty(t, page.NextKey)

	second, _, err := keeper.PaginateAll(ctx, g, &query.PageRequest{Key: page.NextKey, Limit: 2})
	require.NoError(t, err)
	require.Len(t, second, 2)
	require.NotEqual(t, first, second)
}

func TestPaginateByGrantee(t *testing.T) {
	g, ctx, a, b := seeded(t)

	got, _, err := keeper.PaginateByGrantee(ctx, g, a, nil)
	require.NoError(t, err)
	// Grantee is the first key component, so results come back in
	// (action, scope) order.
	require.Equal(t, []types.Grant{
		{Grantee: a, Action: "issue", Scope: scopeA},
		{Grantee: a, Action: "issue", Scope: scopeB},
		{Grantee: a, Action: "type.create"},
	}, got)

	got, _, err = keeper.PaginateByGrantee(ctx, g, b, nil)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{
		{Grantee: b, Action: "revoke", Scope: scopeA},
	}, got)
}

func TestPaginateByGranteeUnknownAddress(t *testing.T) {
	g, ctx, _, _ := seeded(t)

	got, _, err := keeper.PaginateByGrantee(ctx, g, sample.AccAddress(), nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestPaginateByScope(t *testing.T) {
	g, ctx, a, b := seeded(t)

	got, _, err := keeper.PaginateByScope(ctx, g, scopeA, "", nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []types.Grant{
		{Grantee: a, Action: "issue", Scope: scopeA},
		{Grantee: b, Action: "revoke", Scope: scopeA},
	}, got)
}

func TestPaginateByScopeNarrowedToAction(t *testing.T) {
	g, ctx, a, _ := seeded(t)

	got, _, err := keeper.PaginateByScope(ctx, g, scopeA, "issue", nil)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{
		{Grantee: a, Action: "issue", Scope: scopeA},
	}, got)
}

func TestPaginateByScopeEmptyScopeMatchesModuleWideGrants(t *testing.T) {
	g, ctx, a, _ := seeded(t)

	// The empty scope is a real key component, not a wildcard: it selects the
	// module-wide grants and nothing else.
	got, _, err := keeper.PaginateByScope(ctx, g, "", "", nil)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{
		{Grantee: a, Action: "type.create"},
	}, got)
}

func TestGrantFromKey(t *testing.T) {
	grantee := sample.AccAddress()
	g, ctx := newScoped(t)

	require.NoError(t, g.Set(ctx, grantee, "issue", scopeA))

	got, _, err := keeper.PaginateAll(ctx, g, nil)
	require.NoError(t, err)
	require.Equal(t, []types.Grant{{Grantee: grantee, Action: "issue", Scope: scopeA}}, got)
}
