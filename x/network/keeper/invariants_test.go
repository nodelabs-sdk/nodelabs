package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// seedFleet builds a small fleet through the msg server: two active nodes and
// one deactivated, plus a node touched into the next day bucket, so both
// invariants have non-trivial state to check.
func seedFleet(t testing.TB) (*keepertest.NetworkFixture, string) {
	t.Helper()
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 5)

	key := authorizeKey(t, f, ms, f.Ctx, operator)
	nodeA := activateNode(t, f, ms, f.Ctx, key, operator)
	activateNode(t, f, ms, f.Ctx, key, operator)
	gone := activateNode(t, f, ms, f.Ctx, key, operator)

	_, err := ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: gone})
	require.NoError(t, err)

	// Move one node into tomorrow's bucket so the index spans two days.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(24 * time.Hour))
	require.NoError(t, f.Keeper.TouchNodeActivity(later, nodeA))

	return f, operator
}

// TestInvariantsHoldOnHandlerBuiltState: state built only through the msg
// server must satisfy both invariants — that is the whole claim they encode.
func TestInvariantsHoldOnHandlerBuiltState(t *testing.T) {
	f, _ := seedFleet(t)

	require.NoError(t, f.Keeper.CheckInvariants(f.Ctx))
}

// TestInvariantsHoldOnEmptyState: an empty module must not report drift.
func TestInvariantsHoldOnEmptyState(t *testing.T) {
	f, _ := setup(t)

	require.NoError(t, f.Keeper.CheckInvariants(f.Ctx))
}

// TestInvariantsHoldAfterGenesisRebuild: InitGenesis derives both structures
// rather than importing them, so a round-tripped chain must satisfy the same
// invariants — this is what makes the rebuild a repair path.
func TestInvariantsHoldAfterGenesisRebuild(t *testing.T) {
	f, _ := seedFleet(t)

	g, _ := setup(t)
	require.NoError(t, g.Keeper.InitGenesis(g.Ctx, f.Keeper.ExportGenesis(f.Ctx)))

	require.NoError(t, g.Keeper.CheckInvariants(g.Ctx))
}

// TestOperatorNodeCountsInvariantDetectsDrift covers the case the saturating
// decrement in DeactivateNode would otherwise absorb silently: a tally that
// disagrees with the node records in either direction.
func TestOperatorNodeCountsInvariantDetectsDrift(t *testing.T) {
	f, operator := seedFleet(t)
	check := f.Keeper.CheckOperatorNodeCounts

	stored, err := f.Keeper.GetOperatorNodeCounts(f.Ctx, operator, f.NodeType)
	require.NoError(t, err)
	require.Equal(t, uint64(3), stored.Total)
	require.Equal(t, uint64(2), stored.Active)

	// Active undercounted — the shape a missed increment leaves behind.
	drifted := stored
	drifted.Active--
	require.NoError(t, f.Keeper.OperatorNodeCounts.Set(f.Ctx, collections.Join(operator, f.NodeType), drifted))

	err = check(f.Ctx)
	require.Error(t, err, "an undercounted active tally must be reported")
	require.ErrorContains(t, err, "stored {total 3, active 1}")
	require.ErrorContains(t, err, "nodes say {total 3, active 2}")

	// Restored.
	require.NoError(t, f.Keeper.OperatorNodeCounts.Set(f.Ctx, collections.Join(operator, f.NodeType), stored))
	err = check(f.Ctx)
	require.NoError(t, err)

	// Total overcounted — the shape that would inflate an activation limit.
	drifted = stored
	drifted.Total += 10
	require.NoError(t, f.Keeper.OperatorNodeCounts.Set(f.Ctx, collections.Join(operator, f.NodeType), drifted))
	err = check(f.Ctx)
	require.Error(t, err, "an overcounted total must be reported")
	require.ErrorContains(t, err, "stored {total 13, active 2}")

	// A tally removed entirely is drift too, not an absence.
	require.NoError(t, f.Keeper.OperatorNodeCounts.Remove(f.Ctx, collections.Join(operator, f.NodeType)))
	err = check(f.Ctx)
	require.Error(t, err, "a missing tally must be reported")
	require.ErrorContains(t, err, "no stored tally")
}

// TestRecentNodeCountsInvariantDetectsDrift covers the ways a window can
// disagree with the node records, each of which skews the recent-activity
// comparand the activation limit reads.
func TestRecentNodeCountsInvariantDetectsDrift(t *testing.T) {
	f, operator := seedFleet(t)
	check := f.Keeper.CheckRecentNodeCounts
	key := collections.Join(operator, f.NodeType)

	// seedFleet leaves the window on day0+1: nodeA today, the other two
	// (one deactivated) yesterday.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(24 * time.Hour))
	require.NoError(t, check(later))
	good, err := f.Keeper.RecentNodeCounts.Get(f.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, uint64(1), good.Today)
	require.Equal(t, uint64(2), good.Yesterday)

	// 1. An overcount inflates the comparand.
	bad := good
	bad.Today++
	require.NoError(t, f.Keeper.RecentNodeCounts.Set(f.Ctx, key, bad))
	err = check(later)
	require.Error(t, err, "an overcount must be reported")
	require.ErrorContains(t, err, "window on day")

	// 2. An undercount deflates it.
	bad = good
	bad.Yesterday--
	require.NoError(t, f.Keeper.RecentNodeCounts.Set(f.Ctx, key, bad))
	require.Error(t, check(later), "an undercount must be reported")

	// 3. A stale day: right counts, wrong epoch, so the roll lands them in
	// the wrong slots.
	bad = good
	bad.Epoch--
	require.NoError(t, f.Keeper.RecentNodeCounts.Set(f.Ctx, key, bad))
	require.Error(t, check(later), "a window on the wrong day must be reported")

	// 4. A missing window for a pair with recent nodes.
	require.NoError(t, f.Keeper.RecentNodeCounts.Remove(f.Ctx, key))
	err = check(later)
	require.Error(t, err, "a missing window must be reported")
	require.ErrorContains(t, err, "no window")

	// Restored, it passes again; and once every node has aged out, no window
	// at all is also consistent.
	require.NoError(t, f.Keeper.RecentNodeCounts.Set(f.Ctx, key, good))
	require.NoError(t, check(later))
	require.NoError(t, f.Keeper.RecentNodeCounts.Remove(f.Ctx, key))
	require.NoError(t, check(f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(72*time.Hour))))
}

// TestRecentNodeCountsInvariantDetectsOrphanWindow: a window counting nodes
// for a pair that has none. The handlers cannot produce this; the invariant
// covers it because the windows are derived state.
func TestRecentNodeCountsInvariantDetectsOrphanWindow(t *testing.T) {
	f, _ := seedFleet(t)

	ghost := sample.AccAddress()
	require.NoError(t, f.Keeper.RecentNodeCounts.Set(f.Ctx, collections.Join(ghost, f.NodeType),
		types.RecentNodeWindow{Epoch: types.DayEpoch(keepertest.NetworkFixtureBlockTime), Today: 1}))

	err := f.Keeper.CheckRecentNodeCounts(f.Ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, ghost)
}
