package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// TestInitGenesisRunsFullValidation verifies that Keeper.InitGenesis itself
// runs GenesisState.Validate — so direct keeper callers (tests, future
// migrations) get the same invariant enforcement as the JSON path through
// AppModule.ValidateGenesis.
func TestInitGenesisRunsFullValidation(t *testing.T) {
	k, ctx := keepertest.LicenseKeeper(t)

	bad := &types.GenesisState{
		LicenseTypes: []types.LicenseType{
			{
				// Every other field is valid so the negative max_supply is what
				// fails: this test is about InitGenesis running validation at
				// all, not about which rule trips first.
				Id:           "neg",
				MaxSupply:    math.NewInt(-1),
				IssuedCount:  math.ZeroInt(),
				ActiveCount:  math.ZeroInt(),
				RevokedCount: math.ZeroInt(),
			},
		},
	}

	err := k.InitGenesis(ctx, bad)
	require.Error(t, err)
	require.Contains(t, err.Error(), "max_supply must not be negative")
}

// TestGenesisRoundTripActiveIndex verifies that the holder index is rebuilt
// for active licenses only, and that revoked licenses keep their status and
// revoked_date through an export/import cycle.
func TestGenesisRoundTripActiveIndex(t *testing.T) {
	src := keepertest.NewLicenseFixture(t)
	// Revocation stamps revoked_date with the block date; use a realistic
	// block time so the exported dates are meaningful.
	srcCtx := src.Ctx.WithBlockTime(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC))
	owner := src.Owner
	holder := sample.AccAddress()
	ms := keeper.NewMsgServerImpl(src.Keeper)

	_, err := ms.CreateLicenseType(srcCtx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "node", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	src.Grant(t, owner, types.ActionIssue, "node")
	src.Grant(t, owner, types.ActionRevoke, "node")

	resp, err := ms.IssueLicenses(srcCtx, &types.MsgIssueLicenses{
		Issuer: owner, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", Count: 3},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Ids, 3)

	revResp, err := ms.RevokeLicenses(srcCtx, &types.MsgRevokeLicenses{
		Revoker: owner, LicenseTypeId: "node", LicenseIds: []uint64{resp.Ids[2]},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{resp.Ids[2]}, revResp.Ids)

	exported := src.Keeper.ExportGenesis(srcCtx)

	dst, dstCtx := keepertest.LicenseKeeper(t)
	require.NoError(t, dst.InitGenesis(dstCtx, exported))

	// The holder index is rebuilt for active licenses only.
	q := setupQuerier(dst)
	byHolder, err := q.LicensesByHolder(dstCtx, &types.QueryLicensesByHolderRequest{Holder: holder})
	require.NoError(t, err)
	require.Len(t, byHolder.Licenses, 2)

	// The revoked license itself survives with its status, and its end_date
	// carries the revocation date (the source block date).
	l, found, err := dst.GetLicense(dstCtx, resp.Ids[2])
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, types.StatusRevoked, l.Status)
	require.Equal(t, "2026-07-01", l.EndDate)
}

// TestGenesisRoundTripPreservesExplicitCounter verifies that the id sequence
// is genesis state in its own right: a counter deliberately larger than
// issued_count must survive export/import unchanged instead of being
// re-derived from the stats counter.
func TestGenesisRoundTripPreservesExplicitCounter(t *testing.T) {
	src := keepertest.NewLicenseFixture(t)
	srcCtx := src.Ctx
	owner := src.Owner
	holder := sample.AccAddress()
	ms := keeper.NewMsgServerImpl(src.Keeper)

	_, err := ms.CreateLicenseType(srcCtx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "node", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	src.Grant(t, owner, types.ActionIssue, "node")
	_, err = ms.IssueLicenses(srcCtx, &types.MsgIssueLicenses{
		Issuer: owner, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", Count: 2},
		},
	})
	require.NoError(t, err)

	// Bump the sequence past issued_count (2) to simulate the concepts
	// diverging. The stored value is the id to assign next.
	require.NoError(t, src.Keeper.NextLicenseID.Set(srcCtx, 11))

	exported := src.Keeper.ExportGenesis(srcCtx)
	require.Equal(t, uint64(11), exported.NextLicenseId)

	dst := keepertest.NewLicenseFixture(t)
	require.NoError(t, dst.Keeper.InitGenesis(dst.Ctx, exported))
	// Grants are exercised by their own round-trip test below; the
	// destination chain needs its own grant for the issuer.
	dst.Grant(t, owner, types.ActionIssue, "node")

	// The next issued id continues from the explicit counter, not from
	// issued_count.
	dstMs := keeper.NewMsgServerImpl(dst.Keeper)
	resp, err := dstMs.IssueLicenses(dst.Ctx, &types.MsgIssueLicenses{
		Issuer: owner, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node", Holder: holder, StartDate: "2026-02-01", Count: 1},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{11}, resp.Ids)
}

// TestGenesisRoundTripPreservesLicenseIDs covers the LicenseCounts genesis
// export/import path: after exporting and re-importing genesis into a fresh
// keeper, newly issued license IDs must not collide with pre-existing ones.
func TestGenesisRoundTripPreservesLicenseIDs(t *testing.T) {
	// Source keeper: create a license type, issue a handful of licenses,
	// then export the genesis state.
	src := keepertest.NewLicenseFixture(t)
	srcCtx := src.Ctx
	owner := src.Owner
	holder := sample.AccAddress()

	ms := keeper.NewMsgServerImpl(src.Keeper)

	_, err := ms.CreateLicenseType(srcCtx, &types.MsgCreateLicenseType{
		Creator:       owner,
		Id:            "node",
		Transferrable: false,
		MaxSupply:     math.NewInt(100),
	})
	require.NoError(t, err)

	src.Grant(t, owner, types.ActionIssue, "node")

	resp, err := ms.IssueLicenses(srcCtx, &types.MsgIssueLicenses{
		Issuer: owner,
		Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", Count: 5},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2, 3, 4, 5}, resp.Ids)

	exported := src.Keeper.ExportGenesis(srcCtx)

	// Destination keeper: a fresh store, then InitGenesis with the exported
	// state. The genesis must include the existing license type and its
	// counters, so the next issued ID is 6, not 1.
	dst := keepertest.NewLicenseFixture(t)
	dstCtx := dst.Ctx
	require.NoError(t, dst.Keeper.InitGenesis(dstCtx, exported))
	dst.Grant(t, owner, types.ActionIssue, "node")

	// Sanity: the imported license type's IssuedCount survived.
	lt, found, err := dst.Keeper.GetLicenseType(dstCtx, "node")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, math.NewInt(5), lt.IssuedCount, "IssuedCount must survive genesis import")

	// Sanity: the imported licenses survived.
	for _, id := range resp.Ids {
		l, ok, err := dst.Keeper.GetLicense(dstCtx, id)
		require.NoError(t, err)
		require.True(t, ok, "license id %d must exist after import", id)
		require.Equal(t, holder, l.Holder)
	}

	// The bug: nextLicenseID resets to 0 because LicenseCounts isn't
	// restored, so issuing returns id=1 and overwrites the existing one.
	dstMs := keeper.NewMsgServerImpl(dst.Keeper)
	newHolder := sample.AccAddress()
	issueResp, err := dstMs.IssueLicenses(dstCtx, &types.MsgIssueLicenses{
		Issuer: owner,
		Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node", Holder: newHolder, StartDate: "2026-02-01", Count: 1},
		},
	})
	require.NoError(t, err)
	require.Len(t, issueResp.Ids, 1)

	newID := issueResp.Ids[0]
	require.Greater(t, newID, uint64(5), "new license id must not collide with imported ids")

	// And the imported holder's licenses must still all be theirs (i.e., the
	// new issuance didn't overwrite license 1).
	l1, ok, err := dst.Keeper.GetLicense(dstCtx, 1)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, holder, l1.Holder, "imported license 1 must not have been overwritten")
}

// TestGenesisRoundTripParamsAndGrants covers the ownership and access state:
// the module owner and its grants export and re-import with the rest of the
// module, rather than living in a separate genesis section.
func TestGenesisRoundTripParamsAndGrants(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	owner := sample.AccAddress()
	grantee := sample.AccAddress()

	seedType(t, f, "node.license")
	require.NoError(t, f.Keeper.Params.Set(f.Ctx, types.Params{Owner: owner}))
	f.Grant(t, grantee, types.ActionIssue, "node.license")
	f.Grant(t, grantee, types.ActionCreateType, "")

	exported := f.Keeper.ExportGenesis(f.Ctx)
	require.Equal(t, owner, exported.Params.Owner)
	require.ElementsMatch(t, []accesstypes.Grant{
		{Grantee: grantee, Action: types.ActionIssue, Scope: "node.license"},
		{Grantee: grantee, Action: types.ActionCreateType},
		// The fixture self-grants type.create to its own owner.
		{Grantee: f.Owner, Action: types.ActionCreateType},
	}, exported.Grants)

	fresh := keepertest.NewLicenseFixture(t)
	require.NoError(t, fresh.Keeper.InitGenesis(fresh.Ctx, exported))

	params, err := fresh.Keeper.GetParams(fresh.Ctx)
	require.NoError(t, err)
	require.Equal(t, owner, params.Owner)

	has, err := fresh.Keeper.Grants.HasGrant(fresh.Ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.True(t, has)
}

// InitGenesis writes license types before grants, so a grant may name a type
// defined in the same document.
func TestInitGenesisAcceptsGrantOnTypeFromSameDocument(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	grantee := sample.AccAddress()

	require.NoError(t, f.Keeper.InitGenesis(f.Ctx, &types.GenesisState{
		LicenseTypes: []types.LicenseType{{
			Id:           "fresh.license",
			MaxSupply:    math.ZeroInt(),
			IssuedCount:  math.ZeroInt(),
			ActiveCount:  math.ZeroInt(),
			RevokedCount: math.ZeroInt(),
		}},
		NextLicenseId: types.FirstLicenseID,
		Grants: []accesstypes.Grant{
			{Grantee: grantee, Action: types.ActionIssue, Scope: "fresh.license"},
		},
	}))

	has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionIssue, "fresh.license")
	require.NoError(t, err)
	require.True(t, has)
}
