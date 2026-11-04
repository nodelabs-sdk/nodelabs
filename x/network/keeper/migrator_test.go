package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// TestMigrate1to2ReplacesRecentNodeActivity seeds v1 state (the
// RecentNodeActivity index, no windows), runs the migration, and checks the
// windows give the same counts the v1 two-prefix walk did, the invariants
// hold, and the old index is gone.
func TestMigrate1to2ReplacesRecentNodeActivity(t *testing.T) {
	f, ms := setup(t)
	op := sample.AccAddress()
	f.IssueLicenses(t, op, 10)
	f.IssueLicensesOfType(t, op, f.NanoLicenseType, 1)
	key := authorizeKey(t, f, ms, f.Ctx, op)
	at := func(days int) time.Time {
		return keepertest.NetworkFixtureBlockTime.Add(time.Duration(days) * 24 * time.Hour)
	}

	// Day 0: A, B, C (trust) and N (nano). Day 2: touch A, activate then
	// deactivate D. Day 3: touch B. As of day 3: B is today, A and D are
	// yesterday, C and N have aged out.
	a := activateNode(t, f, ms, f.Ctx, key, op)
	b := activateNode(t, f, ms, f.Ctx, key, op)
	activateNode(t, f, ms, f.Ctx, key, op)
	_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{ActivationAddress: key, Operator: op, NodeAddress: sample.AccAddress(), NodeType: f.NanoNodeType})
	require.NoError(t, err)
	day2 := f.WithBlockTime(at(2))
	require.NoError(t, f.Keeper.TouchNodeActivity(day2, a))
	d := activateNode(t, f, ms, day2, key, op)
	_, err = ms.DeactivateNode(day2, &types.MsgDeactivateNode{Signer: op, NodeAddress: d})
	require.NoError(t, err)
	day3 := f.WithBlockTime(at(3))
	require.NoError(t, f.Keeper.TouchNodeActivity(day3, b))

	// Rewrite the store into its v1 shape: one legacy index entry per node,
	// no windows.
	legacy := collections.NewKeySet(collections.NewSchemaBuilder(f.Keeper.StoreService()),
		types.LegacyRecentNodeActivityPrefix, "recent_node_activity",
		collections.QuadKeyCodec(collections.StringKey, collections.StringKey, collections.Uint64Key, collections.StringKey))
	require.NoError(t, f.Keeper.Nodes.Walk(f.Ctx, nil, func(addr string, n types.Node) (bool, error) {
		return false, legacy.Set(f.Ctx, collections.Join4(n.Operator, n.Type, types.DayEpoch(n.LastActiveTime), addr))
	}))
	require.NoError(t, f.Keeper.RecentNodeCounts.Clear(f.Ctx, nil))

	// What the v1 CountRecentActiveNodes walk returned on day 3.
	v1Count := func(nodeType string) uint64 {
		today := types.DayEpoch(at(3))
		var n uint64
		for _, e := range []uint64{today, today - 1} {
			rng := collections.NewSuperPrefixedQuadRange3[string, string, uint64, string](op, nodeType, e)
			require.NoError(t, legacy.Walk(f.Ctx, rng, func(collections.Quad[string, string, uint64, string]) (bool, error) {
				n++
				return false, nil
			}))
		}
		return n
	}
	v1 := map[string]uint64{f.NodeType: v1Count(f.NodeType), f.NanoNodeType: v1Count(f.NanoNodeType)}
	require.Equal(t, uint64(3), v1[f.NodeType])
	require.Equal(t, uint64(0), v1[f.NanoNodeType])

	require.NoError(t, keeper.NewMigrator(f.Keeper).Migrate1to2(day3))

	for _, nt := range []string{f.NodeType, f.NanoNodeType} {
		got, err := f.Keeper.CountRecentActiveNodes(day3, op, nt, at(3))
		require.NoError(t, err)
		require.Equal(t, v1[nt], got, "node type %s", nt)
	}
	require.Equal(t, types.RecentNodeWindow{Epoch: types.DayEpoch(at(3)), Today: 1, Yesterday: 2}, recentWindow(t, f, op, f.NodeType))
	require.NoError(t, f.Keeper.CheckInvariants(day3))

	empty := true
	require.NoError(t, legacy.Walk(f.Ctx, nil, func(collections.Quad[string, string, uint64, string]) (bool, error) {
		empty = false
		return true, nil
	}))
	require.True(t, empty, "legacy RecentNodeActivity entries must be deleted")

	// The state keeps working after the migration: the next day rolls.
	count, err := f.Keeper.CountRecentActiveNodes(day3, op, f.NodeType, at(4))
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
}
