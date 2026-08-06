package keeper

import (
	"context"
	"time"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// TouchNodeActivity records counted activity for a node at the current block
// time: it moves the node's single RecentNodeActivity entry into the current
// day bucket (derived from the stored last_active_time, so the index always
// holds exactly one entry per node) and updates last_active_time. Callers
// are responsible for checking the node is active; the entry-per-node
// invariant must hold for deactivated nodes too.
func (k Keeper) TouchNodeActivity(ctx context.Context, nodeAddr string) error {
	node, err := k.Nodes.Get(ctx, nodeAddr)
	if err != nil {
		return types.ErrNodeNotFound.Wrapf("node %s", nodeAddr)
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime()
	oldEpoch := types.DayEpoch(node.LastActiveTime)
	newEpoch := types.DayEpoch(blockTime)

	if oldEpoch != newEpoch {
		// node.Type is already in hand from the record, so the node-type
		// dimension costs no extra read here.
		if err := k.RecentNodeActivity.Remove(ctx, collections.Join4(node.Operator, node.Type, oldEpoch, nodeAddr)); err != nil {
			return err
		}
		if err := k.RecentNodeActivity.Set(ctx, collections.Join4(node.Operator, node.Type, newEpoch, nodeAddr)); err != nil {
			return err
		}
	}

	node.LastActiveTime = blockTime
	return k.Nodes.Set(ctx, nodeAddr, node)
}

// CountRecentActiveNodes returns the number of the operator's nodes *of one
// node type* with counted activity today or yesterday (UTC): the two-prefix
// walk over RecentNodeActivity. Each node has exactly one index entry, so no
// dedup is needed.
func (k Keeper) CountRecentActiveNodes(ctx context.Context, operator, nodeType string, blockTime time.Time) (uint64, error) {
	today := types.DayEpoch(blockTime)
	var count uint64
	for _, epoch := range []uint64{today, today - 1} {
		rng := collections.NewSuperPrefixedQuadRange3[string, string, uint64, string](operator, nodeType, epoch)
		if err := k.RecentNodeActivity.Walk(ctx, rng, func(_ collections.Quad[string, string, uint64, string]) (bool, error) {
			count++
			return false, nil
		}); err != nil {
			return 0, err
		}
	}
	return count, nil
}

// GetOperatorNodeCounts returns the operator's denormalized node tally for one
// node type, zero-valued when the operator has never activated a node of it.
func (k Keeper) GetOperatorNodeCounts(ctx context.Context, operator, nodeType string) (types.OperatorNodeCounts, error) {
	counts, err := k.OperatorNodeCounts.Get(ctx, collections.Join(operator, nodeType))
	if err != nil {
		return types.OperatorNodeCounts{}, nil
	}
	return counts, nil
}

// countOperatorKeys walks the operator's activation keys and returns how
// many are active and how many were created within the recent-key window
// ending at blockTime (regardless of status: disabled keys keep counting).
func (k Keeper) countOperatorKeys(ctx context.Context, operator string, window time.Duration, blockTime time.Time) (active, recent uint64, err error) {
	cutoff := blockTime.Add(-window)
	rng := collections.NewPrefixedPairRange[string, string](operator)
	err = k.OperatorActivationKeys.Walk(ctx, rng, func(pair collections.Pair[string, string]) (bool, error) {
		key, err := k.ActivationKeys.Get(ctx, pair.K2())
		if err != nil {
			return true, err
		}
		if key.Status == types.KeyActive {
			active++
		}
		if key.CreatedAt.After(cutoff) {
			recent++
		}
		return false, nil
	})
	return active, recent, err
}

// rollCounter advances a daily counter to blockTime: the count resets when
// the stored latest_time falls in a different UTC day.
func rollCounter(c types.ActivityCounter, blockTime time.Time, height int64) types.ActivityCounter {
	if !types.SameDay(c.LatestTime, blockTime) {
		c.DailyCount = 0
	}
	c.LatestTime = blockTime
	c.LatestHeight = height
	return c
}
