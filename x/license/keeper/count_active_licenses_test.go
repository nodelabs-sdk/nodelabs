package keeper_test

import (
	"testing"

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

	// stopAt caps the walk.
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, 1)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, 4)
	require.NoError(t, err)
	require.Equal(t, uint64(4), count)

	// A stopAt above the holdings returns the true count.
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, counted, 100)
	require.NoError(t, err)
	require.Equal(t, uint64(5), count)

	// No counted types means zero without walking.
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, holder, nil, 0)
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
}
