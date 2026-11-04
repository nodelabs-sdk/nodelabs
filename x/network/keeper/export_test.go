package keeper

import (
	"context"

	corestore "cosmossdk.io/core/store"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// Test-only access to unexported helpers, compiled into the package for
// keeper_test.

func RollWindow(w types.RecentNodeWindow, day uint64) types.RecentNodeWindow {
	return rollWindow(w, day)
}

func (k Keeper) MoveRecent(ctx context.Context, operator, nodeType string, oldEpoch *uint64, day uint64) error {
	return k.moveRecent(ctx, operator, nodeType, oldEpoch, day)
}

func (k Keeper) StoreService() corestore.KVStoreService {
	return k.storeService
}
