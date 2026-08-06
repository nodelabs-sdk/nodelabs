package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// recentEntries returns every RecentNodeActivity entry for an operator as
// nodeAddress -> dayEpoch, across all node types and day buckets.
func recentEntries(t testing.TB, f *keepertest.NetworkFixture, operator string) map[string]uint64 {
	t.Helper()
	entries := make(map[string]uint64)
	rng := collections.NewPrefixedQuadRange[string, string, uint64, string](operator)
	require.NoError(t, f.Keeper.RecentNodeActivity.Walk(f.Ctx, rng, func(key collections.Quad[string, string, uint64, string]) (bool, error) {
		entries[key.K4()] = key.K3()
		return false, nil
	}))
	return entries
}

// ---------------------------------------------------------------------------
// Step 0 — authorization
// ---------------------------------------------------------------------------

func TestActivateNodeAuthorization(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	// Unknown activation key.
	_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: sample.AccAddress(),
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// Key bound to a different operator than the msg names.
	other := sample.AccAddress()
	f.IssueLicenses(t, other, 1)
	_, err = ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          other,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// Disabled keys cannot activate.
	_, err = ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: key,
	})
	require.NoError(t, err)
	_, err = ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// ---------------------------------------------------------------------------
// Step 1 — node addresses are single-use
// ---------------------------------------------------------------------------

func TestActivateNodeAddressSingleUse(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 2)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	// Re-activating the same address fails while active...
	_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       node,
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrNodeExists)

	// ...and stays burned after deactivation — no reactivation ever.
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
	require.NoError(t, err)
	_, err = ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       node,
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrNodeExists)

	// Even under a different operator.
	other := sample.AccAddress()
	f.IssueLicenses(t, other, 1)
	otherKey := authorizeKey(t, f, ms, f.Ctx, other)
	_, err = ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: otherKey,
		Operator:          other,
		NodeAddress:       node,
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrNodeExists)
}

// ---------------------------------------------------------------------------
// Steps 2-6 — licenses, limits, spam
// ---------------------------------------------------------------------------

func TestActivateNodeNoLicenses(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	// Authorizing needs no licenses; activating does.
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrNoActiveLicenses)
}

func TestActivateNodeRevokedLicensesStopCounting(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	activateNode(t, f, ms, f.Ctx, key, operator)

	f.RevokeLicenses(t, operator, 1)

	_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrNoActiveLicenses)
}

// TestActivateNodeLimitBoundary: 1 license x multiplier 3 admits exactly
// three concurrently active nodes on the first day.
func TestActivateNodeLimitBoundary(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	for i := 0; i < 3; i++ {
		activateNode(t, f, ms, f.Ctx, key, operator)
	}

	_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrActivationLimit)
	require.ErrorContains(t, err, "(3)")

	// Deactivating one frees an active slot.
	nodes := recentEntries(t, f, operator)
	var anyNode string
	for node := range nodes {
		anyNode = node
		break
	}
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: anyNode})
	require.NoError(t, err)
	activateNode(t, f, ms, f.Ctx, key, operator)
}

// TestActivateNodeSpamLimit drives an activate/deactivate churn cycle to the
// spam ceiling: with 1 license, the ninth node activated within the recent
// window trips spam_limit_multiplier x count = 9 for the tenth, regardless
// of free active slots. Two days later the recent window has drained and
// activation works again.
func TestActivateNodeSpamLimit(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	// Rounds of activate-3 / deactivate-3: each round adds 3 recent
	// entries that deactivation deliberately retains.
	var lastNodes []string
	for round := 0; round < 3; round++ {
		lastNodes = lastNodes[:0]
		for i := 0; i < 3; i++ {
			lastNodes = append(lastNodes, activateNode(t, f, ms, f.Ctx, key, operator))
		}
		if round < 2 {
			for _, node := range lastNodes {
				_, err := ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
				require.NoError(t, err)
			}
		}
	}

	// Nine nodes were activated today; even with active slots free the
	// spam ceiling rejects the tenth.
	_, err := ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: lastNodes[0]})
	require.NoError(t, err)
	_, err = ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrRecentActivationLimit)
	require.ErrorContains(t, err, "(9)")

	// Two days on, the recent window is empty and activation is admitted
	// through the recent-count check despite the all-time total.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(48 * time.Hour))
	_, err = ms.ActivateNode(later, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       sample.AccAddress(),
		NodeType:          f.NodeType,
	})
	require.NoError(t, err)
}

// TestActivateNodePoolsAreIndependentPerNodeType is the heart of per-node-type
// entitlement: a node type is backed only by licenses of the license type it
// is bound to. Holding licenses for one node type grants nothing for another,
// and exhausting one pool leaves the other untouched.
func TestActivateNodePoolsAreIndependentPerNodeType(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()

	// Licensed for trust only; nano is registered by the fixture but unlicensed.
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	activate := func(nodeType string) error {
		_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
			ActivationAddress: key,
			Operator:          operator,
			NodeAddress:       sample.AccAddress(),
			NodeType:          nodeType,
		})
		return err
	}

	// 1 trust licence x multiplier 3 = 3 trust nodes.
	for i := 0; i < 3; i++ {
		require.NoError(t, activate(f.NodeType), "trust node %d", i)
	}
	err := activate(f.NodeType)
	require.ErrorIs(t, err, types.ErrActivationLimit)
	require.ErrorContains(t, err, "(3)")

	// Nano draws on a pool the operator has no licences for, so it is not
	// merely limited — it is unlicensed. Three spare trust licences would not
	// help; there are none to spare here either way.
	err = activate(f.NanoNodeType)
	require.ErrorIs(t, err, types.ErrNoActiveLicenses)
	require.ErrorContains(t, err, f.NanoLicenseType)

	// Licensing nano opens its own pool, and the exhausted trust pool stays
	// exhausted: the two never share.
	f.IssueLicensesOfType(t, operator, f.NanoLicenseType, 1)
	for i := 0; i < 3; i++ {
		require.NoError(t, activate(f.NanoNodeType), "nano node %d", i)
	}
	require.ErrorIs(t, activate(f.NanoNodeType), types.ErrActivationLimit)
	require.ErrorIs(t, activate(f.NodeType), types.ErrActivationLimit)

	// The tallies stayed separate throughout: 3 active of each, not 6 of one.
	for _, nodeType := range []string{f.NodeType, f.NanoNodeType} {
		counts, err := f.Keeper.GetOperatorNodeCounts(f.Ctx, operator, nodeType)
		require.NoError(t, err)
		require.Equal(t, uint64(3), counts.Total, nodeType)
		require.Equal(t, uint64(3), counts.Active, nodeType)
	}
}

// TestActivateNodeTypeRestriction: the node type registry is the allowlist,
// and registration alone is not enough — the operator must also hold licences
// of the type that node type is bound to.
func TestActivateNodeTypeRestriction(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 2)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	activate := func(nodeType string) error {
		_, err := ms.ActivateNode(f.Ctx, &types.MsgActivateNode{
			ActivationAddress: key,
			Operator:          operator,
			NodeAddress:       sample.AccAddress(),
			NodeType:          nodeType,
		})
		return err
	}

	// Never registered.
	require.ErrorIs(t, activate("nodelabs.ghost"), types.ErrInvalidNodeType)

	// Registered, but bound to a licence type the operator holds none of. The
	// operator's trust licences do not carry over to nano.
	require.ErrorIs(t, activate(f.NanoNodeType), types.ErrNoActiveLicenses)

	// Licensed for the bound type: admitted.
	f.IssueLicensesOfType(t, operator, f.NanoLicenseType, 1)
	require.NoError(t, activate(f.NanoNodeType))
}

// ---------------------------------------------------------------------------
// Index-maintenance invariants
// ---------------------------------------------------------------------------

// TestRecentActivityInvariants pins the index contract: exactly one
// RecentNodeActivity entry per node at all times; activation seeds it,
// TouchNodeActivity moves it between day buckets, and deactivation retains
// it (deactivated nodes keep counting in the recent window).
func TestRecentActivityInvariants(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	day0 := types.DayEpoch(keepertest.NetworkFixtureBlockTime)

	// Activation seeds exactly one entry in today's bucket.
	entries := recentEntries(t, f, operator)
	require.Equal(t, map[string]uint64{node: day0}, entries)

	// A same-day touch keeps the single entry in place.
	require.NoError(t, f.Keeper.TouchNodeActivity(f.Ctx, node))
	require.Equal(t, map[string]uint64{node: day0}, recentEntries(t, f, operator))

	// A touch on a later day moves the entry — never duplicates it.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(48 * time.Hour))
	require.NoError(t, f.Keeper.TouchNodeActivity(later, node))
	require.Equal(t, map[string]uint64{node: day0 + 2}, recentEntries(t, f, operator))

	rec, err := f.Keeper.Nodes.Get(f.Ctx, node)
	require.NoError(t, err)
	require.Equal(t, later.BlockTime().Unix(), rec.LastActiveTime.Unix())

	// Deactivation touches neither the entry nor last_active_time.
	evenLater := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(72 * time.Hour))
	_, err = ms.DeactivateNode(evenLater, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
	require.NoError(t, err)
	require.Equal(t, map[string]uint64{node: day0 + 2}, recentEntries(t, f, operator))
	rec, err = f.Keeper.Nodes.Get(f.Ctx, node)
	require.NoError(t, err)
	require.Equal(t, later.BlockTime().Unix(), rec.LastActiveTime.Unix())
}

// TestRecentCountWindow pins CountRecentActiveNodes to the today+yesterday
// two-prefix walk.
func TestRecentCountWindow(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	activateNode(t, f, ms, f.Ctx, key, operator)

	base := keepertest.NetworkFixtureBlockTime

	count, err := f.Keeper.CountRecentActiveNodes(f.Ctx, operator, f.NodeType, base)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// Still visible the next day (yesterday prefix)...
	count, err = f.Keeper.CountRecentActiveNodes(f.Ctx, operator, f.NodeType, base.Add(24*time.Hour))
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)

	// ...and gone the day after.
	count, err = f.Keeper.CountRecentActiveNodes(f.Ctx, operator, f.NodeType, base.Add(48*time.Hour))
	require.NoError(t, err)
	require.Equal(t, uint64(0), count)
}

// ---------------------------------------------------------------------------
// Bookkeeping on activation
// ---------------------------------------------------------------------------

func TestActivateNodeBookkeeping(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	rec, err := f.Keeper.Nodes.Get(f.Ctx, node)
	require.NoError(t, err)
	require.Equal(t, operator, rec.Operator)
	require.Equal(t, key, rec.ActivatedBy)
	require.Equal(t, types.NodeActive, rec.Status)
	require.Equal(t, f.NodeType, rec.Type)

	has, err := f.Keeper.OperatorNodes.Has(f.Ctx, collections.Join(operator, node))
	require.NoError(t, err)
	require.True(t, has)

	counts, err := f.Keeper.GetOperatorNodeCounts(f.Ctx, operator, f.NodeType)
	require.NoError(t, err)
	require.Equal(t, uint64(1), counts.Total)
	require.Equal(t, uint64(1), counts.Active)

	nodeAddr, err := sdk.AccAddressFromBech32(node)
	require.NoError(t, err)
	require.True(t, f.AccountKeeper.HasAccount(f.Ctx, nodeAddr))
}
