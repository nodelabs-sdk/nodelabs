package keeper_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// Same canonicalization guarantee as x/license — this module keys grants on the
// address string too, and its own messages have documented the hazard for node
// and activation addresses since before grants existed.

func TestGrantAccessRejectsNonCanonicalGrantee(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()
	alias := strings.ToUpper(grantee)

	require.ErrorContains(t, (&types.MsgGrantAccess{
		Owner:   f.Owner,
		Grantee: alias,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionWalletCreate}},
	}).ValidateBasic(), "not in canonical form")

	_, err := ms.GrantAccess(f.Ctx, &types.MsgGrantAccess{
		Owner:   f.Owner,
		Grantee: alias,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionWalletCreate}},
	})
	require.ErrorContains(t, err, "not in canonical form")

	for _, key := range []string{alias, grantee} {
		has, err := f.Keeper.Grants.HasGrant(f.Ctx, key, types.ActionWalletCreate, "")
		require.NoError(t, err)
		require.False(t, has, key)
	}
}

func TestRevokeAccessRejectsNonCanonicalGrantee(t *testing.T) {
	f, ms := setupAccess(t)
	grantee := sample.AccAddress()
	f.GrantNetwork(t, grantee, types.ActionWalletCreate)

	_, err := ms.RevokeAccess(f.Ctx, &types.MsgRevokeAccess{
		Owner:   f.Owner,
		Grantee: strings.ToUpper(grantee),
		Actions: []accesstypes.ActionScope{{Action: types.ActionWalletCreate}},
	})
	require.ErrorContains(t, err, "not in canonical form")

	has, err := f.Keeper.Grants.HasGrant(f.Ctx, grantee, types.ActionWalletCreate, "")
	require.NoError(t, err)
	require.True(t, has)
}

func TestTransferOwnershipRejectsNonCanonicalNewOwner(t *testing.T) {
	f, ms := setupAccess(t)

	_, err := ms.TransferOwnership(f.Ctx, &types.MsgTransferOwnership{
		Owner:    f.Owner,
		NewOwner: strings.ToUpper(sample.AccAddress()),
	})
	require.ErrorContains(t, err, "not in canonical form")

	params, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, f.Owner, params.Owner)
}

func TestUpdateParamsRejectsNonCanonicalOwner(t *testing.T) {
	f, ms := setupAccess(t)

	params := types.DefaultParams()
	params.Owner = strings.ToUpper(sample.AccAddress())

	_, err := ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    params,
	})
	require.ErrorContains(t, err, "not in canonical form")
}

func TestGenesisRejectsNonCanonicalGrantee(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Grants = []accesstypes.Grant{
		{Grantee: strings.ToUpper(sample.AccAddress()), Action: types.ActionWalletCreate},
	}
	require.ErrorContains(t, gs.Validate(), "not in canonical form")
}
