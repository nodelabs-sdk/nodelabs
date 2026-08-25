package licenseprecompile

import (
	"bytes"
	_ "embed"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	"cosmossdk.io/core/address"
	"cosmossdk.io/log/v2"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	licensekeeper "github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

var _ vm.PrecompiledContract = &Precompile{}

var (
	// Embed abi json file to the executable binary. Needed when importing as dependency.
	//
	//go:embed abi.json
	f   []byte
	ABI abi.ABI
)

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(f))
	if err != nil {
		panic(err)
	}
}

// Precompile is the EVM precompiled contract that exposes the x/license module.
type Precompile struct {
	cmn.Precompile

	abi.ABI
	keeper      licensekeeper.Keeper
	msgServer   licensetypes.MsgServer
	queryServer licensetypes.QueryServer
	addrCdc     address.Codec
}

// NewPrecompile builds a licenses precompile bound to the given keeper. The
// returned contract is intended to be registered at licensetypes.PrecompileAddress
// (or any operator-chosen address) on the EVM keeper.
func NewPrecompile(
	keeper licensekeeper.Keeper,
	addrCdc address.Codec,
	contractAddress common.Address,
) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.KVGasConfig(),
			TransientKVGasConfig: storetypes.TransientGasConfig(),
			ContractAddress:      contractAddress,
		},
		ABI:         ABI,
		keeper:      keeper,
		msgServer:   licensekeeper.NewMsgServerImpl(keeper),
		queryServer: licensekeeper.NewQuerier(keeper),
		addrCdc:     addrCdc,
	}
}

// Name returns the human-readable identifier of the precompile, required by
// geth 1.17's vm.PrecompiledContract interface.
func (p Precompile) Name() string {
	return "license"
}

// RequiredGas returns the minimum gas required to execute the precompile call.
func (p Precompile) RequiredGas(input []byte) uint64 {
	if len(input) < 4 {
		return 0
	}

	method, err := p.MethodById(input[:4])
	if err != nil {
		return 0
	}

	return p.Precompile.RequiredGas(input, p.IsTransaction(method))
}

// Run executes the precompile, mediating between the EVM and the SDK context.
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, evm.StateDB, contract, readonly)
	})
}

// Execute dispatches to the per-method handlers.
func (p Precompile) Execute(ctx sdk.Context, stateDB vm.StateDB, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	switch method.Name {
	// transactions
	case CreateLicenseTypeMethod:
		return p.CreateLicenseType(ctx, contract, stateDB, method, args)
	case UpdateLicenseTypeMethod:
		return p.UpdateLicenseType(ctx, contract, stateDB, method, args)
	case IssueLicensesMethod:
		return p.IssueLicenses(ctx, contract, stateDB, method, args)
	case RevokeLicensesMethod:
		return p.RevokeLicenses(ctx, contract, stateDB, method, args)

	// queries
	case LicenseTypeMethod:
		return p.LicenseType(ctx, method, args)
	case LicenseTypesMethod:
		return p.LicenseTypes(ctx, method, args)
	case LicenseMethod:
		return p.License(ctx, method, args)
	case LicensesMethod:
		return p.Licenses(ctx, method, args)
	case LicensesByTypeMethod:
		return p.LicensesByType(ctx, method, args)
	case LicensesByHolderMethod:
		return p.LicensesByHolder(ctx, method, args)
	case LicensesByHolderAndTypeMethod:
		return p.LicensesByHolderAndType(ctx, method, args)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}
}

// IsTransaction reports whether a method is a state-writing transaction.
func (Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case CreateLicenseTypeMethod,
		UpdateLicenseTypeMethod,
		IssueLicensesMethod,
		RevokeLicensesMethod:
		return true
	default:
		return false
	}
}

// Logger returns a precompile-scoped logger.
func (p Precompile) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("evm extension", licensetypes.ModuleName)
}
