package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/network/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// setupAccess returns a network fixture and its msg server.
func setupAccess(t testing.TB) (*keepertest.NetworkFixture, types.MsgServer) {
	t.Helper()
	f := keepertest.NewNetworkFixture(t)
	return f, keeper.NewMsgServerImpl(f.Keeper)
}

// ---------------------------------------------------------------------------
// UpdateParams
// ---------------------------------------------------------------------------

func TestUpdateParamsSetsOwner(t *testing.T) {
	f, ms := setupAccess(t)
	newOwner := sample.AccAddress()

	params := types.DefaultParams()
	params.Owner = newOwner

	_, err := ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    params,
	})
	require.NoError(t, err)

	got, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, newOwner, got.Owner)
}

func TestUpdateParamsRejectsNonAuthority(t *testing.T) {
	f, ms := setupAccess(t)

	// Not even the current owner may set params; that is the authority's job.
	_, err := ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: f.Owner,
		Params:    types.DefaultParams(),
	})
	require.ErrorIs(t, err, govtypes.ErrInvalidSigner)

	got, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, f.Owner, got.Owner, "params must be unchanged")
}

func TestUpdateParamsRejectsInvalidOwner(t *testing.T) {
	f, ms := setupAccess(t)

	params := types.DefaultParams()
	params.Owner = "not-an-address"

	_, err := ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    params,
	})
	require.ErrorContains(t, err, "invalid owner address")
}

// ---------------------------------------------------------------------------
// TransferOwnership
// ---------------------------------------------------------------------------

func TestTransferOwnership(t *testing.T) {
	f, ms := setupAccess(t)
	newOwner := sample.AccAddress()

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    f.Owner,
		NewOwner: newOwner,
	})
	require.NoError(t, err)

	got, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, newOwner, got.Owner)

	// The old owner can no longer act.
	_, err = ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    f.Owner,
		NewOwner: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

func TestTransferOwnershipRejectsNonOwner(t *testing.T) {
	f, ms := setupAccess(t)

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    sample.AccAddress(),
		NewOwner: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	got, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, f.Owner, got.Owner)
}

func TestTransferOwnershipRejectsInvalidNewOwner(t *testing.T) {
	f, ms := setupAccess(t)

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    f.Owner,
		NewOwner: "not-an-address",
	})
	require.ErrorIs(t, err, types.ErrInvalidAddress)

	got, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, f.Owner, got.Owner)
}

// TransferOwnership is the one path by which a non-authority signer writes a
// parameter. It must read the current set back and replace only Owner.
//
// This is the load-bearing version of that check: this module's parameters
// govern activation limits and gasless admission, so a handoff that reset any
// of them to a zero value would silently change chain policy — raising the
// gasless caps to "unlimited" or dropping activation limits to zero — with no
// governance proposal and no visible signal.
//
// The parameters below are deliberately set away from both their defaults and
// their zero values, so a handler that wrote a fresh Params{} or a
// DefaultParams() would fail here rather than coincidentally match.
func TestTransferOwnershipPreservesEveryOtherParam(t *testing.T) {
	f, ms := setupAccess(t)

	custom := types.Params{
		ActivationLimitMultiplier: 7,
		SpamLimitMultiplier:       21,
		MaxActivationKeys:         11,
		RecentKeyLimit:            13,
		RecentKeyWindow:           36 * time.Hour,
		StatusDailyLimit:          17,
		DeauthorizeFee:            sdk.NewCoins(sdk.NewInt64Coin("atest", 4200)),
		MaxGaslessGas:             123_456,
		MaxGaslessMsgs:            7,
		MaxGaslessTxBytes:         31_337,
		Owner:                     f.Owner,
	}
	require.NoError(t, custom.Validate(), "the fixture params must themselves be valid")
	require.NoError(t, f.Keeper.Params.Set(f.Ctx, custom))

	newOwner := sample.AccAddress()
	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    f.Owner,
		NewOwner: newOwner,
	})
	require.NoError(t, err)

	got, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)

	want := custom
	want.Owner = newOwner
	require.Equal(t, want, got)

	// Spelled out field by field as well: require.Equal on the struct would
	// still pass if a future field were added and zeroed on both sides, but
	// these assertions name what must survive a handoff.
	require.Equal(t, uint64(7), got.ActivationLimitMultiplier)
	require.Equal(t, uint64(21), got.SpamLimitMultiplier)
	require.Equal(t, uint64(11), got.MaxActivationKeys)
	require.Equal(t, uint64(13), got.RecentKeyLimit)
	require.Equal(t, 36*time.Hour, got.RecentKeyWindow)
	require.Equal(t, uint64(17), got.StatusDailyLimit)
	require.Equal(t, sdk.NewCoins(sdk.NewInt64Coin("atest", 4200)), got.DeauthorizeFee)
	require.Equal(t, uint64(123_456), got.MaxGaslessGas)
	require.Equal(t, uint64(7), got.MaxGaslessMsgs)
	require.Equal(t, uint64(31_337), got.MaxGaslessTxBytes)
}

func TestTransferOwnershipEmitsEvent(t *testing.T) {
	f, ms := setupAccess(t)

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    f.Owner,
		NewOwner: sample.AccAddress(),
	})
	require.NoError(t, err)

	var found bool
	for _, ev := range f.Ctx.EventManager().Events() {
		if ev.Type == accesstypes.EventTypeTransferOwnership {
			found = true
		}
	}
	require.True(t, found, "expected a %s event", accesstypes.EventTypeTransferOwnership)
}

// ---------------------------------------------------------------------------
// Unset owner
// ---------------------------------------------------------------------------

// With no owner configured the module fails closed, and says so specifically.
func TestOwnerGatedMsgsWithNoOwnerSet(t *testing.T) {
	f, ms := setupAccess(t)
	params := types.DefaultParams()
	params.Owner = ""
	require.NoError(t, f.Keeper.Params.Set(f.Ctx, params))

	caller := sample.AccAddress()

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    caller,
		NewOwner: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrOwnerNotSet)

	_, err = ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   caller,
		Grantee: sample.AccAddress(),
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionWalletCreate}},
	})
	require.ErrorIs(t, err, types.ErrOwnerNotSet)

	_, err = ms.RevokeAccess(f.Ctx, &types.MsgRevokeAccess{
		Owner:   caller,
		Grantee: sample.AccAddress(),
		Actions: []accesstypes.ActionScope{{Action: types.ActionWalletCreate}},
	})
	require.ErrorIs(t, err, types.ErrOwnerNotSet)
}

// ---------------------------------------------------------------------------
// GrantAccess / RevokeAccess
// ---------------------------------------------------------------------------

func TestGrantAccess(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()

	_, err := ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   f.Owner,
		Grantee: grantee,
		Grants: []accesstypes.ActionScopes{
			{Action: types.ActionWalletCreate},
			{Action: types.ActionNodeTypeCreate},
		},
	})
	require.NoError(t, err)

	for _, action := range []string{types.ActionWalletCreate, types.ActionNodeTypeCreate} {
		has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, action, "")
		require.NoError(t, err)
		require.True(t, has, action)
	}
}

// The shared grant store would accept a scope here — it treats scopes as
// unconstrained when a module declares no scope validator — and file the grant
// under it, where every check in this module (which looks under the empty
// scope) would miss it. The handler rejects it so that never happens silently.
func TestGrantAccessRejectsScope(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()

	_, err := ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   f.Owner,
		Grantee: grantee,
		Grants: []accesstypes.ActionScopes{
			{Action: types.ActionWalletCreate, Scopes: []string{"anything"}},
		},
	})
	require.ErrorIs(t, err, accesstypes.ErrInvalidScope)

	// Neither the scoped key nor the module-wide one was written.
	for _, scope := range []string{"anything", ""} {
		has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionWalletCreate, scope)
		require.NoError(t, err)
		require.False(t, has, "scope %q", scope)
	}
}

// The same guard on the revoke side: a scoped revoke could not match anything,
// so it is an error rather than a silent no-op.
func TestRevokeAccessRejectsScope(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()
	f.GrantNetwork(t, grantee, types.ActionWalletCreate)

	_, err := ms.RevokeAccess(f.Ctx, &types.MsgRevokeAccess{
		Owner:   f.Owner,
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionWalletCreate, Scope: "anything"}},
	})
	require.ErrorIs(t, err, accesstypes.ErrInvalidScope)

	has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionWalletCreate, "")
	require.NoError(t, err)
	require.True(t, has, "the real grant must be untouched")
}

func TestGrantAccessRejectsNonOwner(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()

	_, err := ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   sample.AccAddress(),
		Grantee: grantee,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionWalletCreate}},
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionWalletCreate, "")
	require.NoError(t, err)
	require.False(t, has)
}

func TestGrantAccessRejectsUnknownAction(t *testing.T) {
	f, ms := setupAccess(t)

	_, err := ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   f.Owner,
		Grantee: sample.AccAddress(),
		Grants:  []accesstypes.ActionScopes{{Action: "node.destroy"}},
	})
	require.ErrorIs(t, err, accesstypes.ErrInvalidAction)
}

func TestRevokeAccess(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()
	f.GrantNetwork(t, grantee, types.ActionWalletCreate)
	f.GrantNetwork(t, grantee, types.ActionNodeTypeCreate)

	_, err := ms.RevokeAccess(f.Ctx, &types.MsgRevokeAccess{
		Owner:   f.Owner,
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionWalletCreate}},
	})
	require.NoError(t, err)

	has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionWalletCreate, "")
	require.NoError(t, err)
	require.False(t, has)

	has, err = f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionNodeTypeCreate, "")
	require.NoError(t, err)
	require.True(t, has, "revoking one action must not touch the others")
}

func TestRevokeAccessRejectsNonOwner(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()
	f.GrantNetwork(t, grantee, types.ActionWalletCreate)

	_, err := ms.RevokeAccess(f.Ctx, &types.MsgRevokeAccess{
		Owner:   sample.AccAddress(),
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionWalletCreate}},
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionWalletCreate, "")
	require.NoError(t, err)
	require.True(t, has, "a rejected revoke must not remove the grant")
}

// The granted right takes effect through the real handler, not just the store.
func TestGrantedNodeTypeRightIsUsable(t *testing.T) {
	f, ms := setupAccess(t)
	creator := sample.AccAddress()

	_, err := ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.extra",
		LicenseTypeId: f.NanoLicenseType,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	_, err = ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   f.Owner,
		Grantee: creator,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionNodeTypeCreate}},
	})
	require.NoError(t, err)

	// Still fails, but now on the binding rule rather than authorization:
	// NanoLicenseType already backs a node type.
	_, err = ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.extra",
		LicenseTypeId: f.NanoLicenseType,
	})
	require.ErrorIs(t, err, types.ErrLicenseTypeBound)
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

func TestQueryActionsAndGrants(t *testing.T) {
	f, _ := setupAccess(t)
	q := keeper.NewQuerier(f.Keeper)
	a, b := sample.AccAddress(), sample.AccAddress()

	f.GrantNetwork(t, a, types.ActionWalletCreate)
	f.GrantNetwork(t, a, types.ActionNodeTypeCreate)
	f.GrantNetwork(t, b, types.ActionWalletCreate)

	actions, err := q.Actions(f.Ctx, &types.QueryActionsRequest{})
	require.NoError(t, err)
	require.Equal(t, []string{types.ActionNodeTypeCreate, types.ActionWalletCreate}, actions.Actions)

	all, err := q.Grants(f.Ctx, &types.QueryGrantsRequest{})
	require.NoError(t, err)
	require.ElementsMatch(t, []accesstypes.Grant{
		{Grantee: a, Action: types.ActionWalletCreate},
		{Grantee: a, Action: types.ActionNodeTypeCreate},
		{Grantee: b, Action: types.ActionWalletCreate},
	}, all.Grants)

	byGrantee, err := q.GrantsByGrantee(f.Ctx, &types.QueryGrantsByGranteeRequest{Grantee: b})
	require.NoError(t, err)
	require.Equal(t, []accesstypes.Grant{
		{Grantee: b, Action: types.ActionWalletCreate},
	}, byGrantee.Grants)

	can, err := q.Can(f.Ctx, &types.QueryCanRequest{Grantee: b, Action: types.ActionWalletCreate})
	require.NoError(t, err)
	require.True(t, can.Can)

	can, err = q.Can(f.Ctx, &types.QueryCanRequest{Grantee: b, Action: types.ActionNodeTypeCreate})
	require.NoError(t, err)
	require.False(t, can.Can)
}

func TestQueryParamsIncludesOwner(t *testing.T) {
	f, _ := setupAccess(t)
	q := keeper.NewQuerier(f.Keeper)

	res, err := q.Params(f.Ctx, &types.QueryParamsRequest{})
	require.NoError(t, err)
	require.Equal(t, f.Owner, res.Params.Owner)
}
