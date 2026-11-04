package keeper

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"cosmossdk.io/collections"
)

// CheckInvariants runs every consistency check over the module's denormalized
// state and reports all problems rather than stopping at the first.
//
// Like x/network's checks, these are plain error-returning methods rather
// than sdk.Invariant: that machinery is deprecated along with x/crisis.
func (k Keeper) CheckInvariants(ctx context.Context) error {
	return k.CheckActiveLicenseCounts(ctx)
}

// CheckActiveLicenseCounts verifies every ActiveLicenseCounts entry equals the
// number of ActiveLicensesByHolder entries under its (holder, license type)
// prefix, and that no prefix with entries lacks a count. The counts are what
// x/network's activation limits read, so drift here admits or refuses
// activations wrongly.
func (k Keeper) CheckActiveLicenseCounts(ctx context.Context) error {
	type holderType struct{ holder, typeID string }
	want := make(map[holderType]uint64)
	if err := k.ActiveLicensesByHolder.Walk(ctx, nil, func(key collections.Triple[string, string, uint64]) (bool, error) {
		want[holderType{key.K1(), key.K2()}]++
		return false, nil
	}); err != nil {
		return fmt.Errorf("active license counts: walking index: %w", err)
	}

	var problems []string
	if err := k.ActiveLicenseCounts.Walk(ctx, nil, func(key collections.Pair[string, string], got uint64) (bool, error) {
		ht := holderType{key.K1(), key.K2()}
		if exp := want[ht]; got != exp {
			problems = append(problems, fmt.Sprintf("\t%s/%s: stored count %d, index holds %d\n", ht.holder, ht.typeID, got, exp))
		}
		delete(want, ht)
		return false, nil
	}); err != nil {
		return fmt.Errorf("active license counts: walking counts: %w", err)
	}

	missing := make([]string, 0, len(want))
	for ht, exp := range want {
		missing = append(missing, fmt.Sprintf("\t%s/%s: no stored count, index holds %d\n", ht.holder, ht.typeID, exp))
	}
	sort.Strings(missing)
	problems = append(problems, missing...)

	if len(problems) > 0 {
		return fmt.Errorf("active license counts: %d problem(s):\n%s", len(problems), strings.Join(problems, ""))
	}
	return nil
}
