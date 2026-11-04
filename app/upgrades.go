package app

import (
	"context"
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

// UpgradeNameDenormalizedCounts runs the license and network v1->v2
// migrations: x/license backfills ActiveLicenseCounts, and x/network replaces
// its RecentNodeActivity index with RecentNodeCounts windows. Together they
// make MsgActivateNode's gas independent of fleet size. Both modules keep
// their store keys (only prefixes inside them change), so no store upgrades
// are needed.
//
// It also raises the network module's max_gasless_gas to
// upgradeMaxGaslessGas; see there.
//
// TODO: rename to the release's upgrade name before proposing it.
const UpgradeNameDenormalizedCounts = "v2-denormalized-counts"

// upgradeMaxGaslessGas is the gasless gas cap this upgrade sets. A
// MsgActivateNode costs ~60.5k gas per message inside a gasless tx plus ~47k
// of per-tx overhead (measured on a localnet), so the old 300k cap admitted
// only 4 activations per tx against max_gasless_msgs = 10. 700k fits 10
// (~652k) with headroom. The cap only bounds a single tx's share of a block;
// gasless spam is bounded by the license-derived admission quotas.
const upgradeMaxGaslessGas = 700_000

func (app NodelabsApp) RegisterUpgradeHandlers() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameDenormalizedCounts,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}
			if err := app.raiseMaxGaslessGas(ctx, upgradeMaxGaslessGas); err != nil {
				return nil, err
			}
			return vm, nil
		},
	)
}

// raiseMaxGaslessGas sets the network module's max_gasless_gas to at least
// floor, leaving every other param untouched. A chain that already runs a
// higher cap keeps it: the upgrade must never tighten a value governance chose.
func (app NodelabsApp) raiseMaxGaslessGas(ctx context.Context, floor uint64) error {
	params, err := app.NetworkKeeper.GetParams(ctx)
	if err != nil {
		return fmt.Errorf("read network params: %w", err)
	}
	if params.MaxGaslessGas >= floor {
		return nil
	}
	params.MaxGaslessGas = floor
	if err := params.Validate(); err != nil {
		return fmt.Errorf("network params after raising max_gasless_gas: %w", err)
	}
	return app.NetworkKeeper.Params.Set(ctx, params)
}
