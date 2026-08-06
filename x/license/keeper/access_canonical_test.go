package keeper_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// The attack this file pins down: an all-uppercase bech32 address decodes to
// the same account and therefore authenticates as it, but grants are keyed on
// the address string. A grant filed under the uppercase alias would be
// exercisable by its holder yet invisible to a revoke or an audit query using
// the canonical form — and RevokeFrom reports a miss as success, so the owner
// would see a successful revoke and an empty grant list while the grant was
// still live.
//
// Every write path that could create such a grant must reject it.

func TestGrantAccessRejectsNonCanonicalGrantee(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")

	grantee := sample.AccAddress()
	alias := strings.ToUpper(grantee)

	// ValidateBasic is the first gate.
	require.ErrorContains(t, (&types.MsgGrantAccess{
		Owner:   owner,
		Grantee: alias,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionIssue, Scopes: []string{"node.license"}}},
	}).ValidateBasic(), "not in canonical form")

	// The handler is gated independently, so a caller reaching the msg server
	// directly is refused too.
	_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   owner,
		Grantee: alias,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionIssue, Scopes: []string{"node.license"}}},
	})
	require.ErrorContains(t, err, "not in canonical form")

	// Neither encoding gained a grant.
	for _, key := range []string{alias, grantee} {
		has, err := f.Keeper.Grants.HasGrant(ctx, key, types.ActionIssue, "node.license")
		require.NoError(t, err)
		require.False(t, has, key)
	}
}

func TestRevokeAccessRejectsNonCanonicalGrantee(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")
	grantee := sample.AccAddress()
	f.Grant(t, grantee, types.ActionIssue, "node.license")

	// RevokeAccess previously validated the grantee not at all, so an alias
	// reached RevokeFrom and returned success having removed nothing.
	_, err := ms.RevokeAccess(ctx, &types.MsgRevokeAccess{
		Owner:   owner,
		Grantee: strings.ToUpper(grantee),
		Actions: []accesstypes.ActionScope{{Action: types.ActionIssue, Scope: "node.license"}},
	})
	require.ErrorContains(t, err, "not in canonical form")

	// The real grant is untouched, and the caller was told the revoke failed
	// rather than being handed a false success.
	has, err := f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.True(t, has)
}

func TestTransferOwnershipRejectsNonCanonicalNewOwner(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	newOwner := sample.AccAddress()

	// Params.Owner is compared by string equality, so an alias here would
	// install an owner that its own key holder could not match.
	_, err := ms.TransferOwnership(ctx, &types.MsgTransferOwnership{
		Owner:    owner,
		NewOwner: strings.ToUpper(newOwner),
	})
	require.ErrorContains(t, err, "not in canonical form")

	params, err := f.Keeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, owner, params.Owner)
}

func TestUpdateParamsRejectsNonCanonicalOwner(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)

	_, err := ms.UpdateParams(ctx, &types.MsgUpdateParams{
		Authority: f.Authority,
		Params:    types.Params{Owner: strings.ToUpper(sample.AccAddress())},
	})
	require.ErrorContains(t, err, "not in canonical form")
}

// Genesis is the remaining write path into the grant store.
func TestGenesisRejectsNonCanonicalGrantee(t *testing.T) {
	gs := types.DefaultGenesis()
	gs.Grants = []accesstypes.Grant{
		{Grantee: strings.ToUpper(sample.AccAddress()), Action: types.ActionCreateType},
	}
	require.ErrorContains(t, gs.Validate(), "not in canonical form")
}

// The canonical form must still work everywhere, or the fix is a regression.
func TestCanonicalAddressesStillAccepted(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	seedType(t, f, "node.license")
	grantee := sample.AccAddress()

	_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
		Owner:   owner,
		Grantee: grantee,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionIssue, Scopes: []string{"node.license"}}},
	})
	require.NoError(t, err)

	has, err := f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.True(t, has)

	_, err = ms.RevokeAccess(ctx, &types.MsgRevokeAccess{
		Owner:   owner,
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionIssue, Scope: "node.license"}},
	})
	require.NoError(t, err)

	has, err = f.Keeper.Grants.HasGrant(ctx, grantee, types.ActionIssue, "node.license")
	require.NoError(t, err)
	require.False(t, has, "a canonical revoke must actually remove the grant")
}
