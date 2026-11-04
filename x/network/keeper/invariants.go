package keeper

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// Consistency checks over the module's denormalized state.
//
// Both recompute a derived structure from the Nodes records — the same
// derivation InitGenesis performs — and report any disagreement. The handlers
// maintain these structures incrementally, so a path that updates a node
// without updating its tally leaves state that is internally inconsistent but
// individually well-formed. DeactivateNode's saturating decrement in
// particular absorbs such a drift silently rather than surfacing it, which is
// what these exist to catch.
//
// These are deliberately NOT built on sdk.Invariant / sdk.InvariantRegistry.
// That machinery is deprecated along with x/crisis as of Cosmos SDK v0.53 and
// is removed in the next release, so a check written against it would stop
// compiling on the next dependency bump. Plain error-returning methods on
// context.Context outlive that, and can be called from tests, a debug query,
// or an upgrade handler alike.

// CheckInvariants runs every consistency check and reports all problems rather
// than stopping at the first, so one pass gives the full picture.
func (k Keeper) CheckInvariants(ctx context.Context) error {
	return errors.Join(
		k.CheckOperatorNodeCounts(ctx),
		k.CheckRecentNodeCounts(ctx),
	)
}

// tallyKey is the (operator, nodeType) pair as a plain comparable value.
//
// It is deliberately not collections.Pair: Pair holds its parts as pointers, so
// as a Go map key it compares by pointer identity and every Join would produce
// a fresh, never-equal key. The same reason InitGenesis declares its own.
type tallyKey struct{ operator, nodeType string }

func (t tallyKey) String() string { return t.operator + "/" + t.nodeType }

// CheckOperatorNodeCounts verifies the denormalized {total, active} tally
// against the node records. Missing entries compare as zero on either side, so
// a stored zero tally and an absent one are equivalent — the handlers never
// write a zero, but treating them alike keeps the comparison total.
func (k Keeper) CheckOperatorNodeCounts(ctx context.Context) error {
	want := make(map[tallyKey]types.OperatorNodeCounts)
	if err := k.Nodes.Walk(ctx, nil, func(_ string, node types.Node) (bool, error) {
		key := tallyKey{operator: node.Operator, nodeType: node.Type}
		c := want[key]
		c.Total++
		if node.Status == types.NodeActive {
			c.Active++
		}
		want[key] = c
		return false, nil
	}); err != nil {
		return fmt.Errorf("operator node counts: walking nodes: %w", err)
	}

	var problems []string

	// Walked in key order, so these lines are already deterministic.
	if err := k.OperatorNodeCounts.Walk(ctx, nil, func(key collections.Pair[string, string], got types.OperatorNodeCounts) (bool, error) {
		tk := tallyKey{operator: key.K1(), nodeType: key.K2()}
		exp := want[tk]
		if got.Total != exp.Total || got.Active != exp.Active {
			problems = append(problems, fmt.Sprintf(
				"\t%s: stored {total %d, active %d}, nodes say {total %d, active %d}\n",
				tk, got.Total, got.Active, exp.Total, exp.Active))
		}
		delete(want, tk)
		return false, nil
	}); err != nil {
		return fmt.Errorf("operator node counts: walking tallies: %w", err)
	}

	// Whatever is left has nodes but no stored tally.
	missing := make([]tallyKey, 0, len(want))
	for tk, exp := range want {
		if exp.Total == 0 && exp.Active == 0 {
			continue
		}
		missing = append(missing, tk)
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].String() < missing[j].String() })
	for _, tk := range missing {
		exp := want[tk]
		problems = append(problems, fmt.Sprintf(
			"\t%s: no stored tally, nodes say {total %d, active %d}\n", tk, exp.Total, exp.Active))
	}

	if len(problems) > 0 {
		return fmt.Errorf("operator node counts: %d tally mismatch(es):\n%s", len(problems), strings.Join(problems, ""))
	}
	return nil
}

// CheckRecentNodeCounts verifies every RecentNodeCounts window against the
// node records: each (operator, nodeType) window must count exactly the nodes
// whose last_active_time falls on its today and yesterday. The window total is
// an activation-limit comparand, so an overcount wrongly refuses activations
// and an undercount wrongly admits them. Deactivated nodes are included:
// deactivation deliberately leaves them counted until their day ages out.
//
// Each window is judged as of the later of the current block day and its own
// stored day, so a check run with an earlier context than the last write
// (possible in tests and queries, never in block execution) compares like
// with like.
func (k Keeper) CheckRecentNodeCounts(ctx context.Context) error {
	blockDay := types.DayEpoch(sdk.UnwrapSDKContext(ctx).BlockTime())

	// Node count per (operator, nodeType) per day of last activity.
	byDay := make(map[tallyKey]map[uint64]uint64)
	if err := k.Nodes.Walk(ctx, nil, func(_ string, node types.Node) (bool, error) {
		tk := tallyKey{operator: node.Operator, nodeType: node.Type}
		if byDay[tk] == nil {
			byDay[tk] = make(map[uint64]uint64)
		}
		byDay[tk][types.DayEpoch(node.LastActiveTime)]++
		return false, nil
	}); err != nil {
		return fmt.Errorf("recent node counts: walking nodes: %w", err)
	}

	var problems []string
	seen := make(map[tallyKey]bool)
	if err := k.RecentNodeCounts.Walk(ctx, nil, func(key collections.Pair[string, string], stored types.RecentNodeWindow) (bool, error) {
		tk := tallyKey{operator: key.K1(), nodeType: key.K2()}
		seen[tk] = true
		day := blockDay
		if stored.Epoch > day {
			day = stored.Epoch
		}
		got := rollWindow(stored, day)
		expToday, expYesterday := byDay[tk][day], byDay[tk][day-1]
		if got.Today != expToday || got.Yesterday != expYesterday {
			problems = append(problems, fmt.Sprintf(
				"\t%s: window on day %d has (today %d, yesterday %d), node records give (today %d, yesterday %d)\n",
				tk, day, got.Today, got.Yesterday, expToday, expYesterday))
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("recent node counts: walking windows: %w", err)
	}

	// A pair with no window must have no node counted in it. Map iteration is
	// unordered, so these are sorted before formatting.
	var missing []string
	for tk, days := range byDay {
		if seen[tk] {
			continue
		}
		for d, n := range days {
			if d+1 >= blockDay {
				missing = append(missing, fmt.Sprintf(
					"\t%s: no window, but %d node(s) were last active on day %d\n", tk, n, d))
			}
		}
	}
	sort.Strings(missing)
	problems = append(problems, missing...)

	if len(problems) > 0 {
		return fmt.Errorf("recent node counts: %d window problem(s):\n%s", len(problems), strings.Join(problems, ""))
	}
	return nil
}
