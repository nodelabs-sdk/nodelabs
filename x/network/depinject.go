package network

import (
	"os"

	"github.com/cosmos/cosmos-sdk/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log/v2"

	modulev1 "github.com/nodelabs-sdk/nodelabs/api/network/module/v1"
	licensekeeper "github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/keeper"
)

var _ appmodule.AppModule = AppModule{}

func init() {
	appmodule.Register(
		&modulev1.Module{},
		appmodule.Provide(ProvideModule),
	)
}

type ModuleInputs struct {
	depinject.In

	Cdc           codec.Codec
	StoreService  store.KVStoreService
	LicenseKeeper licensekeeper.Keeper
	AccountKeeper authkeeper.AccountKeeper
	BankKeeper    bankkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	Module appmodule.AppModule
	Keeper keeper.Keeper
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	govAddr := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	k := keeper.NewKeeper(in.Cdc, in.StoreService, log.NewLogger(os.Stderr), govAddr, in.LicenseKeeper, in.AccountKeeper, in.BankKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{Module: m, Keeper: k}
}
