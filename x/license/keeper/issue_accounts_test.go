package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// TestIssueLicensesCreatesHolderAccount pins the integration contract: a
// wallet holding only a license must have an on-chain account, or it cannot
// sign its first (gasless) transaction.
func TestIssueLicensesCreatesHolderAccount(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	ms := keeper.NewMsgServerImpl(f.Keeper)

	issuer := sample.AccAddress()
	holder := sample.AccAddress()
	existing := sample.AccAddress()

	_, err := ms.CreateLicenseType(f.Ctx, &types.MsgCreateLicenseType{
		Creator:   f.Owner,
		Id:        "node.license",
		MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "node.license")

	holderAddr, err := sdk.AccAddressFromBech32(holder)
	require.NoError(t, err)
	existingAddr, err := sdk.AccAddressFromBech32(existing)
	require.NoError(t, err)

	// Pre-create one holder's account; issuance must not disturb it.
	preexisting := f.AccountKeeper.NewAccountWithAddress(f.Ctx, existingAddr)
	f.AccountKeeper.SetAccount(f.Ctx, preexisting)

	require.False(t, f.AccountKeeper.HasAccount(f.Ctx, holderAddr))

	_, err = ms.IssueLicenses(f.Ctx, &types.MsgIssueLicenses{
		Issuer: issuer,
		Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "node.license", Holder: holder, StartDate: "2025-01-01", Count: 2},
			{LicenseTypeId: "node.license", Holder: existing, StartDate: "2025-01-01", Count: 1},
		},
	})
	require.NoError(t, err)

	// The new holder's account was created; the existing account was reused,
	// not replaced.
	require.True(t, f.AccountKeeper.HasAccount(f.Ctx, holderAddr))
	got := f.AccountKeeper.GetAccount(f.Ctx, existingAddr)
	require.Equal(t, preexisting.GetAccountNumber(), got.GetAccountNumber())
}
