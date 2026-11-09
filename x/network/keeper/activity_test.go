package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/x/network/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

func TestRollWindow(t *testing.T) {
	w := types.RecentNodeWindow{Epoch: 100, Today: 5, Yesterday: 3}

	// Same day: unchanged.
	require.Equal(t, w, keeper.RollWindow(w, 100))
	// Next day: today shifts into yesterday.
	require.Equal(t, types.RecentNodeWindow{Epoch: 101, Today: 0, Yesterday: 5}, keeper.RollWindow(w, 101))
	// Two or more days on: both slots have aged out.
	require.Equal(t, types.RecentNodeWindow{Epoch: 102}, keeper.RollWindow(w, 102))
	require.Equal(t, types.RecentNodeWindow{Epoch: 1000}, keeper.RollWindow(w, 1000))
	// The zero window (never written) rolls to an empty window on any day.
	require.Equal(t, types.RecentNodeWindow{Epoch: 100}, keeper.RollWindow(types.RecentNodeWindow{}, 100))
	// A window ahead of the day asked about is left alone.
	require.Equal(t, w, keeper.RollWindow(w, 99))
}

func TestMoveRecent(t *testing.T) {
	f, _ := setup(t)
	op := sample.AccAddress()
	window := func() types.RecentNodeWindow { return recentWindow(t, f, op, f.NodeType) }
	epoch := func(e uint64) *uint64 { return &e }

	// New nodes (nil old epoch) count into today.
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, nil, 100))
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, nil, 100))
	require.Equal(t, types.RecentNodeWindow{Epoch: 100, Today: 2}, window())

	// Moving a node already in today is a no-op.
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, epoch(100), 100))
	require.Equal(t, types.RecentNodeWindow{Epoch: 100, Today: 2}, window())

	// Next day, a node from yesterday moves into today.
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, epoch(100), 101))
	require.Equal(t, types.RecentNodeWindow{Epoch: 101, Today: 1, Yesterday: 1}, window())

	// A node last counted before yesterday is in neither slot: only added.
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, epoch(90), 101))
	require.Equal(t, types.RecentNodeWindow{Epoch: 101, Today: 2, Yesterday: 1}, window())

	// After a gap of two days the window starts empty.
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, epoch(101), 103))
	require.Equal(t, types.RecentNodeWindow{Epoch: 103, Today: 1}, window())

	// A yesterday underflow (drifted state) saturates instead of failing.
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, epoch(103), 104))
	require.NoError(t, f.Keeper.MoveRecent(f.Ctx, op, f.NodeType, epoch(103), 104))
	require.Equal(t, types.RecentNodeWindow{Epoch: 104, Today: 2}, window())

	// Windows are per node type.
	require.Equal(t, types.RecentNodeWindow{}, recentWindow(t, f, op, f.NanoNodeType))
}
