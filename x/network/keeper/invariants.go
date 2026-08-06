package keeper

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"cosmossdk.io/collections"

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
		k.CheckRecentNodeActivity(ctx),
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

// bucket is a node's expected recent-activity coordinates, as a plain
// comparable value for the same reason tallyKey is.
type bucket struct {
	operator string
	nodeType string
	epoch    uint64
}

// CheckRecentNodeActivity verifies the index holds exactly one entry per node,
// in the day bucket of that node's last_active_time. The count over this index
// is an activation-limit comparand, so a duplicate entry inflates an operator's
// recent-activity figure and a missing one deflates it — and because
// deactivation deliberately leaves entries in place, "one per node" must hold
// for deactivated nodes too.
func (k Keeper) CheckRecentNodeActivity(ctx context.Context) error {
	want := make(map[string]bucket)
	if err := k.Nodes.Walk(ctx, nil, func(addr string, node types.Node) (bool, error) {
		want[addr] = bucket{
			operator: node.Operator,
			nodeType: node.Type,
			epoch:    types.DayEpoch(node.LastActiveTime),
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("recent node activity: walking nodes: %w", err)
	}

	var problems []string
	seen := make(map[string]int, len(want))

	if err := k.RecentNodeActivity.Walk(ctx, nil, func(key collections.Quad[string, string, uint64, string]) (bool, error) {
		addr := key.K4()
		seen[addr]++

		got := bucket{operator: key.K1(), nodeType: key.K2(), epoch: key.K3()}
		exp, known := want[addr]
		switch {
		case !known:
			problems = append(problems, fmt.Sprintf(
				"\t%s: index entry for a node that does not exist (operator %s, type %s, day %d)\n",
				addr, got.operator, got.nodeType, got.epoch))
		case got != exp:
			problems = append(problems, fmt.Sprintf(
				"\t%s: indexed at (operator %s, type %s, day %d), record says (operator %s, type %s, day %d)\n",
				addr, got.operator, got.nodeType, got.epoch, exp.operator, exp.nodeType, exp.epoch))
		}
		return false, nil
	}); err != nil {
		return fmt.Errorf("recent node activity: walking index: %w", err)
	}

	// Map iteration is unordered, so these are sorted before formatting.
	addrs := make([]string, 0, len(want))
	for addr := range want {
		addrs = append(addrs, addr)
	}
	sort.Strings(addrs)
	for _, addr := range addrs {
		switch n := seen[addr]; {
		case n == 0:
			problems = append(problems, fmt.Sprintf("\t%s: node has no index entry\n", addr))
		case n > 1:
			problems = append(problems, fmt.Sprintf("\t%s: node has %d index entries, want exactly 1\n", addr, n))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("recent node activity: %d index problem(s):\n%s", len(problems), strings.Join(problems, ""))
	}
	return nil
}
