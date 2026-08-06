package app

import (
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
)

func (app NodelabsApp) RegisterUpgradeHandlers() {
	// Register upgrade handlers here as needed. Example:
	//
	// app.UpgradeKeeper.SetUpgradeHandler(
	//     "v1.0.0",
	//     func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
	//         return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
	//     },
	// )

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}

	// Example: if upgradeInfo.Name == "v1.0.0" && !app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
	_ = upgradeInfo
	_ = storetypes.StoreUpgrades{}
	_ = upgradetypes.UpgradeStoreLoader
}
