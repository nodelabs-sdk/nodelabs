package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	keepertest "github.com/nodelabs-sdk/nodelabs/x/license/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/testutil/sample"
)

func TestMigrate1to2RebuildsActiveLicenseCounts(t *testing.T) {
	f := keepertest.NewLicenseFixture(t)
	a, b := sample.AccAddress(), sample.AccAddress()
	keepertest.SeedActiveLicenses(t, f.Keeper, f.Ctx, a, "type.a", 3)
	keepertest.SeedActiveLicenses(t, f.Keeper, f.Ctx, a, "type.b", 2)
	keepertest.SeedActiveLicenses(t, f.Keeper, f.Ctx, b, "type.a", 4)

	// Simulate a v1 store: the index exists, the counts do not.
	require.NoError(t, f.Keeper.ActiveLicenseCounts.Clear(f.Ctx, nil))
	count, err := f.Keeper.CountActiveLicenses(f.Ctx, a, []string{"type.a", "type.b"}, 0)
	require.NoError(t, err)
	require.Zero(t, count)

	require.Error(t, f.Keeper.CheckInvariants(f.Ctx))
	require.NoError(t, keeper.NewMigrator(f.Keeper).Migrate1to2(f.Ctx))
	require.NoError(t, f.Keeper.CheckInvariants(f.Ctx))

	count, err = f.Keeper.CountActiveLicenses(f.Ctx, a, []string{"type.a", "type.b"}, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(5), count)
	count, err = f.Keeper.CountActiveLicenses(f.Ctx, b, []string{"type.a"}, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(4), count)
}
