package licenseprecompile

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	cmn "github.com/cosmos/evm/precompiles/common"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// IssueLicenseEntryArg mirrors the Solidity IssueLicenseEntry tuple.
//
// NOTE: this is a type *alias* so it matches the anonymous struct that
// go-ethereum's ABI decoder generates for the corresponding `tuple[]` input.
type IssueLicenseEntryArg = struct {
	LicenseTypeId string         `json:"licenseTypeId"`
	Holder        common.Address `json:"holder"`
	StartDate     string         `json:"startDate"`
	EndDate       string         `json:"endDate"`
	Count         uint64         `json:"count"`
}

// LicenseTypeOutput mirrors the Solidity LicenseType tuple (output side).
type LicenseTypeOutput struct {
	Id            string   `abi:"id"`
	Transferrable bool     `abi:"transferrable"`
	MaxSupply     *big.Int `abi:"maxSupply"`
	IssuedCount   *big.Int `abi:"issuedCount"`
	ActiveCount   *big.Int `abi:"activeCount"`
	RevokedCount  *big.Int `abi:"revokedCount"`
}

// LicenseOutput mirrors the Solidity License tuple (output side).
type LicenseOutput struct {
	Id        uint64         `abi:"id"`
	TypeId    string         `abi:"typeId"`
	Holder    common.Address `abi:"holder"`
	StartDate string         `abi:"startDate"`
	EndDate   string         `abi:"endDate"`
	Status    string         `abi:"status"`
}

// hexToBech32 converts an EVM address to its bech32 form using the chain's account codec.
func hexToBech32(addrCdc address.Codec, hex common.Address) (string, error) {
	bech, err := addrCdc.BytesToString(hex.Bytes())
	if err != nil {
		return "", fmt.Errorf("invalid address %s: %w", hex.Hex(), err)
	}
	return bech, nil
}

// bech32ToHex converts a bech32 account address to its 20-byte EVM form. An
// empty input returns the zero address with no error (the legitimate
// "unset" case). Any non-empty input that fails to decode returns an error so
// callers can surface state corruption instead of silently emitting a
// zero-address holder.
func bech32ToHex(bech string) (common.Address, error) {
	if bech == "" {
		return common.Address{}, nil
	}
	accAddr, err := sdk.AccAddressFromBech32(bech)
	if err != nil {
		return common.Address{}, fmt.Errorf("invalid bech32 address %q: %w", bech, err)
	}
	return common.BytesToAddress(accAddr.Bytes()), nil
}

// bigIntFromCosmosInt converts a (possibly nil) cosmos math.Int to a *big.Int,
// returning zero when the value is uninitialised.
func bigIntFromCosmosInt(v math.Int) *big.Int {
	if v.IsNil() {
		return new(big.Int)
	}
	return v.BigInt()
}

// licenseTypeToOutput converts an SDK LicenseType into its ABI counterpart.
// Every field is a string or a counter, so unlike licenseToOutput there is no
// address to decode and the conversion cannot fail.
func licenseTypeToOutput(lt licensetypes.LicenseType) LicenseTypeOutput {
	return LicenseTypeOutput{
		Id:            lt.Id,
		Transferrable: lt.Transferrable,
		MaxSupply:     bigIntFromCosmosInt(lt.MaxSupply),
		IssuedCount:   bigIntFromCosmosInt(lt.IssuedCount),
		ActiveCount:   bigIntFromCosmosInt(lt.ActiveCount),
		RevokedCount:  bigIntFromCosmosInt(lt.RevokedCount),
	}
}

// licenseToOutput converts an SDK License into its ABI counterpart. Returns
// an error if the stored holder bech32 is malformed.
func licenseToOutput(l licensetypes.License) (LicenseOutput, error) {
	holder, err := bech32ToHex(l.Holder)
	if err != nil {
		return LicenseOutput{}, fmt.Errorf("license (type=%s, id=%d): %w", l.Type, l.Id, err)
	}
	return LicenseOutput{
		Id:        l.Id,
		TypeId:    l.Type,
		Holder:    holder,
		StartDate: l.StartDate,
		EndDate:   l.EndDate,
		Status:    l.Status.Short(),
	}, nil
}

// licensesToOutputs converts a slice of SDK Licenses into their ABI
// counterparts, propagating the first decode error.
func licensesToOutputs(ls []licensetypes.License) ([]LicenseOutput, error) {
	out := make([]LicenseOutput, 0, len(ls))
	for _, l := range ls {
		o, err := licenseToOutput(l)
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

// argToString unwraps a generic ABI argument expected to be a string.
func argToString(arg interface{}, name string) (string, error) {
	v, ok := arg.(string)
	if !ok {
		return "", fmt.Errorf("invalid type for %s: expected string, got %T", name, arg)
	}
	return v, nil
}

// argToAddress unwraps a generic ABI argument expected to be a common.Address.
func argToAddress(arg interface{}, name string) (common.Address, error) {
	v, ok := arg.(common.Address)
	if !ok {
		return common.Address{}, fmt.Errorf("invalid type for %s: expected address, got %T", name, arg)
	}
	return v, nil
}

// argToBool unwraps a generic ABI argument expected to be a bool.
func argToBool(arg interface{}, name string) (bool, error) {
	v, ok := arg.(bool)
	if !ok {
		return false, fmt.Errorf("invalid type for %s: expected bool, got %T", name, arg)
	}
	return v, nil
}

// argToBigInt unwraps a generic ABI argument expected to be a *big.Int.
func argToBigInt(arg interface{}, name string) (*big.Int, error) {
	v, ok := arg.(*big.Int)
	if !ok || v == nil {
		return nil, fmt.Errorf("invalid type for %s: expected uint256, got %T", name, arg)
	}
	return v, nil
}

// argToUint64 unwraps a generic ABI argument expected to be a uint64.
func argToUint64(arg interface{}, name string) (uint64, error) {
	v, ok := arg.(uint64)
	if !ok {
		return 0, fmt.Errorf("invalid type for %s: expected uint64, got %T", name, arg)
	}
	return v, nil
}

// argCount validates that the right number of arguments were passed.
func argCount(args []interface{}, want int) error {
	if len(args) != want {
		return fmt.Errorf(cmn.ErrInvalidNumberOfArgs, want, len(args))
	}
	return nil
}
