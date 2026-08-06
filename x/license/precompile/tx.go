package licenseprecompile

import (
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/core/vm"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// Transaction method names. Must match the function names in LicenseI.sol / abi.json.
const (
	CreateLicenseTypeMethod = "createLicenseType"
	UpdateLicenseTypeMethod = "updateLicenseType"
	IssueLicensesMethod     = "issueLicenses"
	RevokeLicensesMethod    = "revokeLicenses"
)

// CreateLicenseType handles the createLicenseType ABI method.
func (p Precompile) CreateLicenseType(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if err := argCount(args, 3); err != nil {
		return nil, err
	}
	id, err := argToString(args[0], "id")
	if err != nil {
		return nil, err
	}
	transferrable, err := argToBool(args[1], "transferrable")
	if err != nil {
		return nil, err
	}
	maxSupply, err := argToBigInt(args[2], "maxSupply")
	if err != nil {
		return nil, err
	}

	creator, err := hexToBech32(p.addrCdc, contract.Caller())
	if err != nil {
		return nil, err
	}

	msg := &licensetypes.MsgCreateLicenseType{
		Creator:       creator,
		Id:            id,
		Transferrable: transferrable,
		MaxSupply:     math.NewIntFromBigInt(maxSupply),
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if _, err := p.msgServer.CreateLicenseType(ctx, msg); err != nil {
		return nil, err
	}

	if err := p.EmitLicenseTypeCreated(ctx, stateDB, id, transferrable, maxSupply); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

// UpdateLicenseType handles the updateLicenseType ABI method.
func (p Precompile) UpdateLicenseType(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if err := argCount(args, 2); err != nil {
		return nil, err
	}
	id, err := argToString(args[0], "id")
	if err != nil {
		return nil, err
	}
	transferrable, err := argToBool(args[1], "transferrable")
	if err != nil {
		return nil, err
	}

	owner, err := hexToBech32(p.addrCdc, contract.Caller())
	if err != nil {
		return nil, err
	}

	msg := &licensetypes.MsgUpdateLicenseType{
		Owner:         owner,
		Id:            id,
		Transferrable: transferrable,
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	if _, err := p.msgServer.UpdateLicenseType(ctx, msg); err != nil {
		return nil, err
	}

	if err := p.EmitLicenseTypeUpdated(ctx, stateDB, id, transferrable); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

// IssueLicenses handles the issueLicenses ABI method.
func (p Precompile) IssueLicenses(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if err := argCount(args, 1); err != nil {
		return nil, err
	}

	rawEntries, ok := args[0].([]IssueLicenseEntryArg)
	if !ok {
		return nil, fmt.Errorf("invalid type for entries: expected IssueLicenseEntry[], got %T", args[0])
	}

	issuer, err := hexToBech32(p.addrCdc, contract.Caller())
	if err != nil {
		return nil, err
	}

	entries := make([]licensetypes.IssueLicenseEntry, 0, len(rawEntries))
	for i, e := range rawEntries {
		holder, err := hexToBech32(p.addrCdc, e.Holder)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		entries = append(entries, licensetypes.IssueLicenseEntry{
			LicenseTypeId: e.LicenseTypeId,
			Holder:        holder,
			StartDate:     e.StartDate,
			EndDate:       e.EndDate,
			Count:         e.Count,
		})
	}

	msg := &licensetypes.MsgIssueLicenses{
		Issuer:  issuer,
		Entries: entries,
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	res, err := p.msgServer.IssueLicenses(ctx, msg)
	if err != nil {
		return nil, err
	}

	for _, e := range rawEntries {
		if err := p.EmitLicenseIssued(ctx, stateDB, contract.Caller(), e.Holder, e.LicenseTypeId, e.Count); err != nil {
			return nil, err
		}
	}

	return method.Outputs.Pack(res.Ids)
}

// RevokeLicenses handles the revokeLicenses ABI method.
func (p Precompile) RevokeLicenses(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	if err := argCount(args, 2); err != nil {
		return nil, err
	}
	licenseTypeID, err := argToString(args[0], "licenseTypeId")
	if err != nil {
		return nil, err
	}
	licenseIDs, ok := args[1].([]uint64)
	if !ok {
		return nil, fmt.Errorf("invalid type for licenseIds: expected uint64[], got %T", args[1])
	}

	revoker, err := hexToBech32(p.addrCdc, contract.Caller())
	if err != nil {
		return nil, err
	}

	msg := &licensetypes.MsgRevokeLicenses{
		Revoker:       revoker,
		LicenseTypeId: licenseTypeID,
		LicenseIds:    licenseIDs,
	}

	if err := msg.ValidateBasic(); err != nil {
		return nil, err
	}

	res, err := p.msgServer.RevokeLicenses(ctx, msg)
	if err != nil {
		return nil, err
	}

	if err := p.EmitLicenseRevoked(ctx, stateDB, contract.Caller(), licenseTypeID, res.Ids); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(res.Ids)
}
