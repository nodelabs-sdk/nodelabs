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

// TestRecentNodeActivityInvariantDetectsDrift covers the three ways the
// one-entry-per-node invariant can break, each of which skews the
// recent-activity comparand the activation limit reads.
func TestRecentNodeActivityInvariantDetectsDrift(t *testing.T) {
	f, operator := seedFleet(t)
	check := f.Keeper.CheckRecentNodeActivity

	// Pick a node and its true bucket.
	var addr string
	var node types.Node
	require.NoError(t, f.Keeper.Nodes.Walk(f.Ctx, nil, func(a string, n types.Node) (bool, error) {
		addr, node = a, n
		return true, nil
	}))
	epoch := types.DayEpoch(node.LastActiveTime)
	trueKey := collections.Join4(operator, f.NodeType, epoch, addr)

	// 1. A duplicate entry in another day bucket inflates the count.
	require.NoError(t, f.Keeper.RecentNodeActivity.Set(f.Ctx, collections.Join4(operator, f.NodeType, epoch+1, addr)))
	err := check(f.Ctx)
	require.Error(t, err, "a duplicate index entry must be reported")
	require.ErrorContains(t, err, "2 index entries, want exactly 1")

	require.NoError(t, f.Keeper.RecentNodeActivity.Remove(f.Ctx, collections.Join4(operator, f.NodeType, epoch+1, addr)))
	err = check(f.Ctx)
	require.NoError(t, err)

	// 2. A missing entry deflates it.
	require.NoError(t, f.Keeper.RecentNodeActivity.Remove(f.Ctx, trueKey))
	err = check(f.Ctx)
	require.Error(t, err, "a missing index entry must be reported")
	require.ErrorContains(t, err, "no index entry")

	// 3. An entry in the wrong bucket: present and unique, but stale — the
	// failure a TouchNodeActivity that moved one key and not the other leaves.
	require.NoError(t, f.Keeper.RecentNodeActivity.Set(f.Ctx, collections.Join4(operator, f.NodeType, epoch-5, addr)))
	err = check(f.Ctx)
	require.Error(t, err, "an entry in the wrong day bucket must be reported")
	require.ErrorContains(t, err, "record says")
}

// TestRecentNodeActivityInvariantDetectsOrphanEntry: an index entry naming a
// node that no longer exists. Node records are never removed, so this cannot
// arise from the handlers — the invariant covers it because the index is
// rebuilt from genesis, where the two can be supplied independently.
func TestRecentNodeActivityInvariantDetectsOrphanEntry(t *testing.T) {
	f, operator := seedFleet(t)

	ghost := sample.AccAddress()
	require.NoError(t, f.Keeper.RecentNodeActivity.Set(f.Ctx,
		collections.Join4(operator, f.NodeType, types.DayEpoch(keepertest.NetworkFixtureBlockTime), ghost)))

	err := f.Keeper.CheckRecentNodeActivity(f.Ctx)
	require.Error(t, err)
	require.ErrorContains(t, err, "index entry for a node that does not exist")
}
