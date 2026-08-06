package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

func setupQuerier(k keeper.Keeper) keeper.Querier {
	return keeper.NewQuerier(k)
}

func TestQueryLicenseType(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	q := setupQuerier(f.Keeper)

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "node", Transferrable: true, MaxSupply: math.NewInt(50),
	})
	require.NoError(t, err)

	resp, err := q.LicenseType(ctx, &types.QueryLicenseTypeRequest{Id: "node"})
	require.NoError(t, err)
	require.Equal(t, "node", resp.LicenseType.Id)
	require.True(t, resp.LicenseType.Transferrable)
	require.Equal(t, math.NewInt(50), resp.LicenseType.MaxSupply)

	// Not found
	_, err = q.LicenseType(ctx, &types.QueryLicenseTypeRequest{Id: "missing"})
	require.Error(t, err)
}

func TestQueryLicenseTypes(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	q := setupQuerier(f.Keeper)

	for _, id := range []string{"a", "b", "c"} {
		_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
			Creator: owner, Id: id, MaxSupply: math.ZeroInt(),
		})
		require.NoError(t, err)
	}

	resp, err := q.LicenseTypes(ctx, &types.QueryLicenseTypesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.LicenseTypes, 3)
}

func TestQueryLicense(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	q := setupQuerier(f.Keeper)
	issuer := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "ql", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "ql")

	issueResp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "ql", Holder: holder, StartDate: "2026-01-01", Count: 1},
		},
	})
	require.NoError(t, err)

	resp, err := q.License(ctx, &types.QueryLicenseRequest{Id: issueResp.Ids[0]})
	require.NoError(t, err)
	require.Equal(t, holder, resp.License.Holder)
	require.Equal(t, types.StatusActive, resp.License.Status)

	// Not found
	_, err = q.License(ctx, &types.QueryLicenseRequest{Id: 999})
	require.Error(t, err)
}

// TestQueryLicenses covers the all-licenses query: it spans license types,
// includes revoked records, and paginates without gaps or duplicates.
func TestQueryLicenses(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	q := setupQuerier(f.Keeper)
	admin := sample.AccAddress()
	holder := sample.AccAddress()

	for _, id := range []string{"a1", "b2"} {
		_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
			Creator: owner, Id: id, MaxSupply: math.ZeroInt(),
		})
		require.NoError(t, err)
	}
	f.Grant(t, admin, types.ActionIssue, "a1")
	f.Grant(t, admin, types.ActionIssue, "b2")
	f.Grant(t, admin, types.ActionRevoke, "a1")

	issueResp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: admin, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "a1", Holder: holder, StartDate: "2026-01-01", Count: 3},
			{LicenseTypeId: "b2", Holder: holder, StartDate: "2026-01-01", Count: 2},
		},
	})
	require.NoError(t, err)
	_, err = ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: admin, LicenseTypeId: "a1", LicenseIds: []uint64{issueResp.Ids[0]},
	})
	require.NoError(t, err)

	// Full listing includes revoked licenses.
	resp, err := q.Licenses(ctx, &types.QueryLicensesRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Licenses, 5)

	// Page through with limit 2: three pages, five records, no duplicates.
	var all []types.License
	var nextKey []byte
	pages := 0
	for {
		page, err := q.Licenses(ctx, &types.QueryLicensesRequest{
			Pagination: &query.PageRequest{Key: nextKey, Limit: 2},
		})
		require.NoError(t, err)
		all = append(all, page.Licenses...)
		pages++
		nextKey = page.Pagination.NextKey
		if len(nextKey) == 0 {
			break
		}
	}
	require.Equal(t, 3, pages)
	require.Len(t, all, 5)

	// Dedup on the id alone: ids are unique chain-wide, so this catches two
	// types colliding on an id as well as a paging bug repeating a record.
	seen := make(map[uint64]struct{}, len(all))
	ids := make([]uint64, 0, len(all))
	for _, l := range all {
		_, dup := seen[l.Id]
		require.False(t, dup, "no duplicate ids across pages or types")
		seen[l.Id] = struct{}{}
		ids = append(ids, l.Id)
	}

	// Three of a1 then two of b2, from one sequence, walked in id order.
	require.Equal(t, []uint64{1, 2, 3, 4, 5}, ids)
}

func TestQueryLicensesByHolder(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	q := setupQuerier(f.Keeper)
	issuer := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "h1", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	_, err = ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "h2", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "h1")
	f.Grant(t, issuer, types.ActionIssue, "h2")

	// Issue 2 of h1 and 1 of h2 to holder, and 1 of h1 to someone else
	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "h1", Holder: holder, StartDate: "2026-01-01", Count: 2},
			{LicenseTypeId: "h2", Holder: holder, StartDate: "2026-01-01", Count: 1},
			{LicenseTypeId: "h1", Holder: sample.AccAddress(), StartDate: "2026-01-01", Count: 1},
		},
	})
	require.NoError(t, err)

	resp, err := q.LicensesByHolder(ctx, &types.QueryLicensesByHolderRequest{Holder: holder})
	require.NoError(t, err)
	require.Len(t, resp.Licenses, 3)

	// Filter by holder and type
	resp2, err := q.LicensesByHolderAndType(ctx, &types.QueryLicensesByHolderAndTypeRequest{
		Holder: holder, TypeId: "h1",
	})
	require.NoError(t, err)
	require.Len(t, resp2.Licenses, 2)

	// Pagination over the holder's 3 active licenses: page boundaries stay
	// within the holder prefix (the other holder's license never appears).
	page1, err := q.LicensesByHolder(ctx, &types.QueryLicensesByHolderRequest{
		Holder: holder, Pagination: &query.PageRequest{Limit: 2},
	})
	require.NoError(t, err)
	require.Len(t, page1.Licenses, 2)
	require.NotEmpty(t, page1.Pagination.NextKey)

	page2, err := q.LicensesByHolder(ctx, &types.QueryLicensesByHolderRequest{
		Holder: holder, Pagination: &query.PageRequest{Key: page1.Pagination.NextKey, Limit: 2},
	})
	require.NoError(t, err)
	require.Len(t, page2.Licenses, 1)
	require.Empty(t, page2.Pagination.NextKey)
	for _, l := range append(page1.Licenses, page2.Licenses...) {
		require.Equal(t, holder, l.Holder)
	}

	// LicensesByType paginates over the (type, id) keyspace: h1 has 3
	// licenses total across both holders.
	typePage, err := q.LicensesByType(ctx, &types.QueryLicensesByTypeRequest{
		TypeId: "h1", Pagination: &query.PageRequest{Limit: 2},
	})
	require.NoError(t, err)
	require.Len(t, typePage.Licenses, 2)
	require.NotEmpty(t, typePage.Pagination.NextKey)

	// LicensesByHolderAndType honors pagination too.
	htPage, err := q.LicensesByHolderAndType(ctx, &types.QueryLicensesByHolderAndTypeRequest{
		Holder: holder, TypeId: "h1", Pagination: &query.PageRequest{Limit: 1},
	})
	require.NoError(t, err)
	require.Len(t, htPage.Licenses, 1)
	require.NotEmpty(t, htPage.Pagination.NextKey)
}

// TestQueryLicensesByHolderExcludesRevoked: the holder index tracks active
// licenses only, so holder queries must not return revoked licenses.
func TestQueryLicensesByHolderExcludesRevoked(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	q := setupQuerier(f.Keeper)
	admin := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "rvq", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, admin, types.ActionIssue, "rvq")
	f.Grant(t, admin, types.ActionRevoke, "rvq")

	issueResp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: admin, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "rvq", Holder: holder, StartDate: "2026-01-01", Count: 3},
		},
	})
	require.NoError(t, err)

	_, err = ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: admin, LicenseTypeId: "rvq", LicenseIds: []uint64{issueResp.Ids[2]},
	})
	require.NoError(t, err)

	resp, err := q.LicensesByHolder(ctx, &types.QueryLicensesByHolderRequest{Holder: holder})
	require.NoError(t, err)
	require.Len(t, resp.Licenses, 2)
	for _, l := range resp.Licenses {
		require.Equal(t, types.StatusActive, l.Status)
	}

	respBoth, err := q.LicensesByHolderAndType(ctx, &types.QueryLicensesByHolderAndTypeRequest{
		Holder: holder, TypeId: "rvq",
	})
	require.NoError(t, err)
	require.Len(t, respBoth.Licenses, 2)

	// The revoked license is still reachable by direct lookup.
	lresp, err := q.License(ctx, &types.QueryLicenseRequest{Id: issueResp.Ids[2]})
	require.NoError(t, err)
	require.Equal(t, types.StatusRevoked, lresp.License.Status)
}
