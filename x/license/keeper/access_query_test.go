package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

func TestQueryParams(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.Params(f.Ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, f.Owner, res.Params.Owner)
}

// An owner that was never set reads back empty rather than erroring: "no owner
// configured yet" is a legitimate chain state, not a failure.
func TestQueryParamsWithNoOwnerSet(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	require.NoError(t, f.Keeper.Params.Set(f.Ctx, types.Params{}))
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.Params(f.Ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Empty(t, res.Params.Owner)
}

func TestQueryActions(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.Actions(f.Ctx, &types.QueryActionsRequest{})
	require.NoError(t, err)
	require.Equal(t, []string{types.ActionIssue, types.ActionRevoke, types.ActionCreateType}, res.Actions)
	require.Equal(t, []string{types.ActionCreateType}, res.Unscoped)
}

// seedGrants sets up two license types and a spread of grants across two
// grantees, returning both addresses.
func seedGrants(t testing.TB, f *keepertest.LicenseFixture) (string, string) {
	t.Helper()
	seedType(t, f, "node.license")
	seedType(t, f, "validator.license")

	a, b := sample.AccAddress(), sample.AccAddress()
	f.Grant(t, a, types.ActionIssue, "node.license")
	f.Grant(t, a, types.ActionIssue, "validator.license")
	f.Grant(t, a, types.ActionCreateType, "")
	f.Grant(t, b, types.ActionRevoke, "node.license")
	return a, b
}

func TestQueryGrants(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	a, b := seedGrants(t, f)
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.Grants(f.Ctx, &types.QueryGrantsRequest{})
	require.NoError(t, err)
	require.Subset(t, res.Grants, []accesstypes.Grant{
		{Grantee: a, Action: types.ActionIssue, Scope: "node.license"},
		{Grantee: a, Action: types.ActionIssue, Scope: "validator.license"},
		{Grantee: a, Action: types.ActionCreateType},
		{Grantee: b, Action: types.ActionRevoke, Scope: "node.license"},
	})
}

func TestQueryGrantsByGrantee(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	a, b := seedGrants(t, f)
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.GrantsByGrantee(f.Ctx, &types.QueryGrantsByGranteeRequest{Grantee: b})
	require.NoError(t, err)
	require.Equal(t, []accesstypes.Grant{
		{Grantee: b, Action: types.ActionRevoke, Scope: "node.license"},
	}, res.Grants)

	res, err = q.GrantsByGrantee(f.Ctx, &types.QueryGrantsByGranteeRequest{Grantee: a})
	require.NoError(t, err)
	require.Len(t, res.Grants, 3)
}

func TestQueryGrantsByScope(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	a, b := seedGrants(t, f)
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.GrantsByScope(f.Ctx, &types.QueryGrantsByScopeRequest{Scope: "node.license"})
	require.NoError(t, err)
	require.ElementsMatch(t, []accesstypes.Grant{
		{Grantee: a, Action: types.ActionIssue, Scope: "node.license"},
		{Grantee: b, Action: types.ActionRevoke, Scope: "node.license"},
	}, res.Grants)

	res, err = q.GrantsByScope(f.Ctx, &types.QueryGrantsByScopeRequest{
		Scope:  "node.license",
		Action: types.ActionIssue,
	})
	require.NoError(t, err)
	require.Equal(t, []accesstypes.Grant{
		{Grantee: a, Action: types.ActionIssue, Scope: "node.license"},
	}, res.Grants)
}

func TestQueryCan(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	a, _ := seedGrants(t, f)
	q := keeper.NewQuerier(f.Keeper)

	for _, tc := range []struct {
		action, scope string
		want          bool
	}{
		{types.ActionIssue, "node.license", true},
		{types.ActionRevoke, "node.license", false},
		{types.ActionCreateType, "", true},
		// A module-wide grant is filed under the empty scope only, so looking
		// it up under a real license type must miss.
		{types.ActionCreateType, "node.license", false},
	} {
		res, err := q.Can(f.Ctx, &types.QueryCanRequest{Grantee: a, Action: tc.action, Scope: tc.scope})
		require.NoError(t, err)
		require.Equal(t, tc.want, res.Can, "%s/%s", tc.action, tc.scope)
	}
}
