package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"

	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

func TestCountActiveLicenses(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	holder := sample.AccAddress()
	other := sample.AccAddress()

	seed := func(holder, typeID string, count uint64) {
		t.Helper()
		keepertest.SeedActiveLicenses(t, f.Keeper, f.Ctx, holder, typeID, count)
	}

	seed(holder, "type.a", 3)
	seed(holder, "type.b", 2)
	seed(holder, "type.uncounted", 4)
	seed(other, "type.a", 5)

	counted := []string{"type.a", "type.b"}

	// Full count aggregates across the requested types only, per holder.
	count, err := f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(5), count)

	// stopAt is ignored: the exact count comes back whatever it is.
	for _, stopAt := range []uint64{1, 4, 100} {
		count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, stopAt)
		require.NoError(t, err)
		require.Equal(t, uint64(5), count, "stopAt %d", stopAt)
	}

	// No counted types means zero.
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, nil, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)

	// A holder with none of a type counts zero.
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, other, []string{"type.b"}, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)

	// Revoked licenses drop out of the count (the index holds active only).
	f.Grant(t, other, types.ActionRevoke, "type.a")
	// Revoke two of holder's type.a licenses via the msg path. Seeded ids are
	// sequential from FirstLicenseID, so holder's type.a licenses are 1..3.
	msgServer := keeper.NewMsgServerImpl(f.Keeper)
	_, err = msgServer.RevokeLicenses(f.Ctx, &types.MsgRevokeLicenses{
		Revoker:       other,
		LicenseTypeId: "type.a",
		LicenseIds:    []uint64{types.FirstLicenseID, types.FirstLicenseID + 1},
	})
	require.NoError(t, err)

	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(3), count)

	// Revoking every license of a type removes its count entry entirely.
	_, err = msgServer.RevokeLicenses(f.Ctx, &types.MsgRevokeLicenses{
		Revoker:       other,
		LicenseTypeId: "type.a",
		LicenseIds:    []uint64{types.FirstLicenseID + 2},
	})
	require.NoError(t, err)
	has, err := f.Keeper.ActiveLicenseCounts.Has(f.Ctx, collections.Join(holder, "type.a"))
	require.NoError(t, err)
	require.False(t, has)
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(2), count)
}
