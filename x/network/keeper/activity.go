package keeper

import (
	"context"
	"errors"
	"sort"
	"time"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// TouchNodeActivity records counted activity for a node at the current block
// time: on the node's first counted activity of a UTC day it moves the node
// into today's slot of its (operator, nodeType) RecentNodeCounts window, then
// updates last_active_time. The node's previous day is derived from the stored
// last_active_time, so each node is counted at most once. Callers are
// responsible for checking the node is active.
//
// This is the only path that may advance a node's last_active_time: the window
// is maintained here, so writing the field elsewhere would desynchronize it.
func (k Keeper) TouchNodeActivity(ctx context.Context, nodeAddr string) error {
	node, err := k.Nodes.Get(ctx, nodeAddr)
	if err != nil {
		return types.ErrNodeNotFound.Wrapf("node %s", nodeAddr)
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime()
	oldEpoch := types.DayEpoch(node.LastActiveTime)
	newEpoch := types.DayEpoch(blockTime)

	if oldEpoch != newEpoch {
		if err := k.moveRecent(ctx, node.Operator, node.Type, &oldEpoch, newEpoch); err != nil {
			return err
		}
	}

	node.LastActiveTime = blockTime
	return k.Nodes.Set(ctx, nodeAddr, node)
}

// rollWindow advances w so that its today slot refers to day. Advancing by one
// day shifts today into yesterday; advancing further empties both slots.
//
// A window ahead of day (w.Epoch > day) is returned unchanged. Block time
// never moves backwards, so state writes cannot produce one; it can only be
// seen by a read made with an earlier time than the last write.
func rollWindow(w types.RecentNodeWindow, day uint64) types.RecentNodeWindow {
	switch {
	case w.Epoch >= day:
		return w
	case w.Epoch+1 == day:
		w.Yesterday = w.Today
		w.Today = 0
	default:
		w.Yesterday = 0
		w.Today = 0
	}
	w.Epoch = day
	return w
}

// getRecentWindow returns the stored window for (operator, nodeType), the zero
// window when there is none. It is not rolled.
func (k Keeper) getRecentWindow(ctx context.Context, operator, nodeType string) (types.RecentNodeWindow, error) {
	w, err := k.RecentNodeCounts.Get(ctx, collections.Join(operator, nodeType))
	if errors.Is(err, collections.ErrNotFound) {
		return types.RecentNodeWindow{}, nil
	}
	return w, err
}

// moveRecent counts one node into today's slot of its (operator, nodeType)
// window. oldEpoch is the day the node was previously counted under, nil for a
// node that was never counted (activation). A node previously counted
// yesterday is taken out of yesterday's slot; one counted earlier than that is
// in neither slot, so there is nothing to take out. Calling it with
// *oldEpoch == day is a no-op: the node is already in today.
//
// The yesterday decrement saturates rather than failing. An underflow means
// the window has drifted from the node records; failing here would block every
// status update for the operator, while CheckRecentNodeCounts reports it.
func (k Keeper) moveRecent(ctx context.Context, operator, nodeType string, oldEpoch *uint64, day uint64) error {
	if oldEpoch != nil && *oldEpoch == day {
		return nil
	}
	w, err := k.getRecentWindow(ctx, operator, nodeType)
	if err != nil {
		return err
	}
	w = rollWindow(w, day)
	if oldEpoch != nil && *oldEpoch+1 == day && w.Yesterday > 0 {
		w.Yesterday--
	}
	w.Today++
	return k.RecentNodeCounts.Set(ctx, collections.Join(operator, nodeType), w)
}

// CountRecentActiveNodes returns the number of the operator's nodes *of one
// node type* with counted activity today or yesterday (UTC) relative to
// blockTime: one read of the RecentNodeCounts window, rolled forward. Cost is
// constant in fleet size.
func (k Keeper) CountRecentActiveNodes(ctx context.Context, operator, nodeType string, blockTime time.Time) (uint64, error) {
	w, err := k.getRecentWindow(ctx, operator, nodeType)
	if err != nil {
		return 0, err
	}
	w = rollWindow(w, types.DayEpoch(blockTime))
	return w.Today + w.Yesterday, nil
}

// recentWindows accumulates RecentNodeCounts windows from node records as of
// one day, for rebuilding the map (genesis import and the v1->v2 migration).
// Nodes last active on any day other than day or day-1 are in neither slot.
type recentWindows struct {
	day     uint64
	windows map[tallyKey]types.RecentNodeWindow
}

func newRecentWindows(day uint64) *recentWindows {
	return &recentWindows{day: day, windows: make(map[tallyKey]types.RecentNodeWindow)}
}

func (r *recentWindows) add(node types.Node) {
	epoch := types.DayEpoch(node.LastActiveTime)
	if epoch != r.day && epoch+1 != r.day {
		return
	}
	key := tallyKey{operator: node.Operator, nodeType: node.Type}
	w := r.windows[key]
	w.Epoch = r.day
	if epoch == r.day {
		w.Today++
	} else {
		w.Yesterday++
	}
	r.windows[key] = w
}

// write stores every accumulated window in sorted key order, so the writes are
// deterministic across validators.
func (r *recentWindows) write(ctx context.Context, k Keeper) error {
	keys := make([]tallyKey, 0, len(r.windows))
	for key := range r.windows {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].operator != keys[j].operator {
			return keys[i].operator < keys[j].operator
		}
		return keys[i].nodeType < keys[j].nodeType
	})
	for _, key := range keys {
		if err := k.RecentNodeCounts.Set(ctx, collections.Join(key.operator, key.nodeType), r.windows[key]); err != nil {
			return err
		}
	}
	return nil
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
