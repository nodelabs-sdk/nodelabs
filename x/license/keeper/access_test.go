package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// seedType creates a license type directly, so grant scopes have something
// real to name.
func seedType(t testing.TB, f *keepertest.LicenseFixture, id string) {
	t.Helper()
	require.NoError(t, f.Keeper.LicenseTypes.Set(f.Ctx, id, types.LicenseType{
		Id:           id,
		MaxSupply:    math.ZeroInt(),
		IssuedCount:  math.ZeroInt(),
		ActiveCount:  math.ZeroInt(),
		RevokedCount: math.ZeroInt(),
	}))
}

// ---------------------------------------------------------------------------
// UpdateParams
// ---------------------------------------------------------------------------

func TestUpdateParamsSetsOwner(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)
	newOwner := sample.AccAddress()

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    types.Params{Owner: newOwner},
	})
	require.NoError(t, err)

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, newOwner, params.Owner)
}

func TestUpdateParamsRejectsNonAuthority(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)

	// Not even the current owner may set params; that is the authority's job.
	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: owner,
		Params:    types.Params{Owner: sample.AccAddress()},
	})
	require.ErrorIs(t, err, govtypes.ErrInvalidSigner)

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, owner, params.Owner, "params must be unchanged")
}

func TestUpdateParamsRejectsInvalidOwner(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    types.Params{Owner: "not-an-address"},
	})
	require.ErrorContains(t, err, "invalid owner address")
}

// UpdateParams is the recovery path: it must work with no current owner, which
// is the state a chain is in before governance first sets one.
func TestUpdateParamsEstablishesFirstOwner(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)
	require.NoError(t, f.Keeper.Params.Set(ctx, types.Params{}))

	owner := sample.AccAddress()
	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    types.Params{Owner: owner},
	})
	require.NoError(t, err)

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, owner, params.Owner)
}

// ---------------------------------------------------------------------------
// TransferOwnership
// ---------------------------------------------------------------------------

func TestTransferOwnership(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	newOwner := sample.AccAddress()

	_, err := ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    owner,
		NewOwner: newOwner,
	})
	require.NoError(t, err)

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, newOwner, params.Owner)

	// The old owner can no longer act.
	_, err = ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    owner,
		NewOwner: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

func TestTransferOwnershipRejectsNonOwner(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)

	_, err := ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    sample.AccAddress(),
		NewOwner: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, owner, params.Owner)
}

func TestTransferOwnershipRejectsInvalidNewOwner(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)

	_, err := ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    owner,
		NewOwner: "not-an-address",
	})
	require.ErrorContains(t, err, "invalid new owner address")

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, owner, params.Owner)
}

// TransferOwnership is the one path by which a non-authority signer writes a
// parameter. It must read the current set back and replace only Owner, never
// reset the rest to their zero values.
//
// The license parameter set is currently just Owner, so this pins the handler's
// read-modify-write shape rather than catching a live regression; the same
// guarantee is load-bearing in x/network, whose parameter set is large.
func TestTransferOwnershipWritesOnlyTheOwnerField(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	newOwner := sample.AccAddress()

	before, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)

	_, err = ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    owner,
		NewOwner: newOwner,
	})
	require.NoError(t, err)

	after, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)

	want := before
	want.Owner = newOwner
	require.Equal(t, want, after)
}

func TestTransferOwnershipEmitsEvent(t *testing.T) {
	_, ms, ctx, owner := setupWithOwner(t)

	_, err := ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    owner,
		NewOwner: sample.AccAddress(),
	})
	require.NoError(t, err)

	var found bool
	for _, ev := range ctx.EventManager().Events() {
		if ev.Type == accesstypes.EventTypeTransferOwnership {
			found = true
		}
	}
	require.True(t, found, "expected a %s event", accesstypes.EventTypeTransferOwnership)
}

// ---------------------------------------------------------------------------
// Unset owner
// ---------------------------------------------------------------------------

// With no owner configured the module fails closed, and says so specifically:
// callers can tell "not the owner" from "no owner configured".
func TestOwnerGatedMsgsWithNoOwnerSet(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	ms := keeper.NewMsgServerImpl(f.Keeper)
	require.NoError(t, f.Keeper.Params.Set(f.Ctx, types.Params{}))

	caller := sample.AccAddress()

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    caller,
		NewOwner: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrOwnerNotSet)

	_, err = ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   caller,
		Grantee: sample.AccAddress(),
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionCreateType}},
	})
	require.ErrorIs(t, err, types.ErrOwnerNotSet)

	_, err = ms.UpdateLicenseType(f.Ctx, &types.MsgUpdateLicenseType{
		Owner: caller,
		Id:    "whatever",
	})
	require.ErrorIs(t, err, types.ErrOwnerNotSet)
}

// ---------------------------------------------------------------------------
// GrantAccess / RevokeAccess
// ---------------------------------------------------------------------------

func TestGrantAccess(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")
	grantee := sample.AccAddress()

	_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   owner,
		Grantee: grantee,
		Grants: []accesstypes.ActionScopes{
			{Action: types.ActionIssue, Scopes: []string{"node.license"}},
		},
	})
	require.NoError(t, err)

	has, err := f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.True(t, has)
}

func TestGrantAccessRejectsNonOwner(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)
	seedType(t, f, "node.license")
	grantee := sample.AccAddress()

	_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   sample.AccAddress(),
		Grantee: grantee,
		Grants: []accesstypes.ActionScopes{
			{Action: types.ActionIssue, Scopes: []string{"node.license"}},
		},
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	has, err := f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.False(t, has)
}

func TestGrantAccessRejectsUnknownLicenseType(t *testing.T) {
	_, ms, ctx, owner := setupWithOwner(t)

	_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   owner,
		Grantee: sample.AccAddress(),
		Grants: []accesstypes.ActionScopes{
			{Action: types.ActionIssue, Scopes: []string{"nonexistent"}},
		},
	})
	require.ErrorIs(t, err, accesstypes.ErrInvalidScope)
}

func TestGrantAccessRejectsInvalidGrantee(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")

	_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   owner,
		Grantee: "not-an-address",
		Grants: []accesstypes.ActionScopes{
			{Action: types.ActionIssue, Scopes: []string{"node.license"}},
		},
	})
	require.ErrorContains(t, err, "invalid grantee address")
}

func TestRevokeAccess(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")
	grantee := sample.AccAddress()
	f.Grant(t, grantee, types.ActionIssue, "node.license")

	_, err := ms.RevokeAccess(ctx, &types.MsgRevokeAccess{
		Owner:   owner,
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionIssue, Scope: "node.license"}},
	})
	require.NoError(t, err)

	has, err := f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.False(t, has)
}

func TestRevokeAccessRejectsNonOwner(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)
	seedType(t, f, "node.license")
	grantee := sample.AccAddress()
	f.Grant(t, grantee, types.ActionIssue, "node.license")

	_, err := ms.RevokeAccess(ctx, &types.MsgRevokeAccess{
		Owner:   sample.AccAddress(),
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionIssue, Scope: "node.license"}},
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	has, err := f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.True(t, has, "a rejected revoke must not remove the grant")
}

// The granted right takes effect through the real handler, not just the store.
func TestGrantedIssueRightIsUsable(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")
	issuer := sample.AccAddress()

	_, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer,
		Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node.license", Holder: sample.AccAddress(), StartDate: "2025-01-01", Count: 1},
		},
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	_, err = ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   owner,
		Grantee: issuer,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionIssue, Scopes: []string{"node.license"}}},
	})
	require.NoError(t, err)

	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer,
		Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node.license", Holder: sample.AccAddress(), StartDate: "2025-01-01", Count: 1},
		},
	})
	require.NoError(t, err)
}
