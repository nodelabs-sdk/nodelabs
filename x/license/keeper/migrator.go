package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Migrator handles in-place store migrations for the license module.
type Migrator struct {
	keeper Keeper
}

func NewMigrator(k Keeper) Migrator {
	return Migrator{keeper: k}
}

// Migrate1to2 backfills ActiveLicenseCounts from the ActiveLicensesByHolder
// index. The counts were introduced in v2 so CountActiveLicenses no longer
// walks a holder's licenses.
func (m Migrator) Migrate1to2(ctx sdk.Context) error {
	return m.keeper.RebuildActiveLicenseCounts(ctx)
}
