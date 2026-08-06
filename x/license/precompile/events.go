package licenseprecompile

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Event names. Must match the event names in LicenseI.sol / abi.json.
const (
	EventTypeLicenseTypeCreated = "LicenseTypeCreated"
	EventTypeLicenseTypeUpdated = "LicenseTypeUpdated"
	EventTypeLicenseIssued      = "LicenseIssued"
	EventTypeLicenseRevoked     = "LicenseRevoked"
)

// emitLog writes an EVM log entry to the stateDB.
func (p Precompile) emitLog(ctx sdk.Context, stateDB vm.StateDB, topics []common.Hash, data []byte) {
	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        data,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115: block height is non-negative.
	})
}

// packArgs ABI-encodes the non-indexed event inputs.
func packArgs(event abi.Event, values ...interface{}) ([]byte, error) {
	nonIndexed := event.Inputs.NonIndexed()
	return nonIndexed.Pack(values...)
}

// EmitLicenseTypeCreated emits the LicenseTypeCreated event.
func (p Precompile) EmitLicenseTypeCreated(ctx sdk.Context, stateDB vm.StateDB, id string, transferrable bool, maxSupply *big.Int) error {
	event := p.Events[EventTypeLicenseTypeCreated]

	idTopic, err := cmn.MakeTopic(id)
	if err != nil {
		return err
	}

	data, err := packArgs(event, transferrable, maxSupply)
	if err != nil {
		return err
	}

	p.emitLog(ctx, stateDB, []common.Hash{event.ID, idTopic}, data)
	return nil
}

// EmitLicenseTypeUpdated emits the LicenseTypeUpdated event.
func (p Precompile) EmitLicenseTypeUpdated(ctx sdk.Context, stateDB vm.StateDB, id string, transferrable bool) error {
	event := p.Events[EventTypeLicenseTypeUpdated]

	idTopic, err := cmn.MakeTopic(id)
	if err != nil {
		return err
	}

	data, err := packArgs(event, transferrable)
	if err != nil {
		return err
	}

	p.emitLog(ctx, stateDB, []common.Hash{event.ID, idTopic}, data)
	return nil
}

// EmitLicenseIssued emits the LicenseIssued event.
func (p Precompile) EmitLicenseIssued(ctx sdk.Context, stateDB vm.StateDB, issuer, holder common.Address, licenseTypeID string, count uint64) error {
	event := p.Events[EventTypeLicenseIssued]

	issuerTopic, err := cmn.MakeTopic(issuer)
	if err != nil {
		return err
	}
	holderTopic, err := cmn.MakeTopic(holder)
	if err != nil {
		return err
	}

	data, err := packArgs(event, licenseTypeID, count)
	if err != nil {
		return err
	}

	p.emitLog(ctx, stateDB, []common.Hash{event.ID, issuerTopic, holderTopic}, data)
	return nil
}

// EmitLicenseRevoked emits the LicenseRevoked event.
func (p Precompile) EmitLicenseRevoked(ctx sdk.Context, stateDB vm.StateDB, revoker common.Address, licenseTypeID string, licenseIDs []uint64) error {
	event := p.Events[EventTypeLicenseRevoked]

	revokerTopic, err := cmn.MakeTopic(revoker)
	if err != nil {
		return err
	}

	data, err := packArgs(event, licenseTypeID, licenseIDs)
	if err != nil {
		return err
	}

	p.emitLog(ctx, stateDB, []common.Hash{event.ID, revokerTopic}, data)
	return nil
}
