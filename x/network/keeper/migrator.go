package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// Migrator handles in-place store migrations for the network module.
type Migrator struct {
	keeper Keeper
}

func NewMigrator(k Keeper) Migrator {
	return Migrator{keeper: k}
}

// Migrate1to2 replaces the v1 RecentNodeActivity index (one key per node
// under prefix 13) with the v2 RecentNodeCounts windows. Windows are built
// from the node records as of the upgrade block's day, the same derivation
// InitGenesis performs, and then every key under the old prefix is deleted.
func (m Migrator) Migrate1to2(ctx sdk.Context) error {
	recent := newRecentWindows(types.DayEpoch(ctx.BlockTime()))
	if err := m.keeper.Nodes.Walk(ctx, nil, func(_ string, node types.Node) (bool, error) {
		recent.add(node)
		return false, nil
	}); err != nil {
		return err
	}
	if err := recent.write(ctx, m.keeper); err != nil {
		return err
	}
	return m.keeper.deletePrefix(ctx, types.LegacyRecentNodeActivityPrefix)
}

// deletePrefix removes every key under prefix from the module store. Keys are
// collected before deleting so the iterator is never invalidated by a write.
func (k Keeper) deletePrefix(ctx context.Context, prefix []byte) error {
	store := k.storeService.OpenKVStore(ctx)
	end := prefixEnd(prefix)
	iter, err := store.Iterator(prefix, end)
	if err != nil {
		return err
	}
	var keys [][]byte
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, iter.Key())
	}
	if err := iter.Close(); err != nil {
		return err
	}
	for _, key := range keys {
		if err := store.Delete(key); err != nil {
			return err
		}
	}
	return nil
}

// prefixEnd returns the exclusive upper bound of all keys starting with
// prefix: prefix with its last non-0xff byte incremented, nil if none.
func prefixEnd(prefix []byte) []byte {
	end := append([]byte(nil), prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] < 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return nil
}
