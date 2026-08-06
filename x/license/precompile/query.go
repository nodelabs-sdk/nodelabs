package licenseprecompile

import (
	"github.com/ethereum/go-ethereum/accounts/abi"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// Query method names. Must match the function names in LicenseI.sol / abi.json.
const (
	LicenseTypeMethod             = "licenseType"
	LicenseTypesMethod            = "licenseTypes"
	LicenseMethod                 = "license"
	LicensesMethod                = "licenses"
	LicensesByTypeMethod          = "licensesByType"
	LicensesByHolderMethod        = "licensesByHolder"
	LicensesByHolderAndTypeMethod = "licensesByHolderAndType"
)

// LicenseType returns a single license type by id.
func (p Precompile) LicenseType(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 1); err != nil {
		return nil, err
	}
	id, err := argToString(args[0], "id")
	if err != nil {
		return nil, err
	}

	res, err := p.queryServer.LicenseType(ctx, &licensetypes.QueryLicenseTypeRequest{Id: id})
	if err != nil {
		return nil, err
	}

	return method.Outputs.Pack(licenseTypeToOutput(res.LicenseType))
}

// LicenseTypes returns all license types. It walks the keeper directly: the
// gRPC handler paginates with a default page limit an EVM call cannot page
// through.
func (p Precompile) LicenseTypes(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 0); err != nil {
		return nil, err
	}

	var out []LicenseTypeOutput
	if err := p.keeper.LicenseTypes.Walk(ctx, nil, func(_ string, lt licensetypes.LicenseType) (bool, error) {
		out = append(out, licenseTypeToOutput(lt))
		return false, nil
	}); err != nil {
		return nil, err
	}
	return method.Outputs.Pack(out)
}

// License returns a single license by id. Ids are unique chain-wide, so no
// license type is needed to resolve one.
func (p Precompile) License(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 1); err != nil {
		return nil, err
	}
	id, err := argToUint64(args[0], "id")
	if err != nil {
		return nil, err
	}

	res, err := p.queryServer.License(ctx, &licensetypes.QueryLicenseRequest{Id: id})
	if err != nil {
		return nil, err
	}

	out, err := licenseToOutput(res.License)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(out)
}

// Licenses returns every license across all license types, active and
// revoked. It walks the keeper directly so the result is not capped by the
// gRPC default page limit; gas metering bounds the walk.
func (p Precompile) Licenses(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 0); err != nil {
		return nil, err
	}

	var licenses []licensetypes.License
	if err := p.keeper.Licenses.Walk(ctx, nil, func(_ uint64, l licensetypes.License) (bool, error) {
		licenses = append(licenses, l)
		return false, nil
	}); err != nil {
		return nil, err
	}

	out, err := licensesToOutputs(licenses)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(out)
}

// LicensesByType returns all licenses of a given type.
func (p Precompile) LicensesByType(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 1); err != nil {
		return nil, err
	}
	typeID, err := argToString(args[0], "typeId")
	if err != nil {
		return nil, err
	}

	// Walk the by-type index directly: the gRPC handler paginates with a
	// default page limit, which an EVM call cannot page through.
	var licenses []licensetypes.License
	rng := collections.NewPrefixedPairRange[string, uint64](typeID)
	if err := p.keeper.LicensesByType.Walk(ctx, rng, func(key collections.Pair[string, uint64]) (bool, error) {
		l, err := p.keeper.Licenses.Get(ctx, key.K2())
		if err != nil {
			return true, err
		}
		licenses = append(licenses, l)
		return false, nil
	}); err != nil {
		return nil, err
	}

	out, err := licensesToOutputs(licenses)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(out)
}

// LicensesByHolder returns all licenses held by the given holder.
func (p Precompile) LicensesByHolder(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 1); err != nil {
		return nil, err
	}
	holderHex, err := argToAddress(args[0], "holder")
	if err != nil {
		return nil, err
	}
	holder, err := hexToBech32(p.addrCdc, holderHex)
	if err != nil {
		return nil, err
	}

	licenses, err := p.activeLicensesForHolder(ctx, collections.NewPrefixedTripleRange[string, string, uint64](holder))
	if err != nil {
		return nil, err
	}

	out, err := licensesToOutputs(licenses)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(out)
}

// activeLicensesForHolder walks the active-licenses holder index over the
// given range and loads the full license records. Used instead of the gRPC
// handlers, which paginate with a default page limit an EVM call cannot page
// through.
func (p Precompile) activeLicensesForHolder(ctx sdk.Context, rng collections.Ranger[collections.Triple[string, string, uint64]]) ([]licensetypes.License, error) {
	var licenses []licensetypes.License
	err := p.keeper.ActiveLicensesByHolder.Walk(ctx, rng, func(key collections.Triple[string, string, uint64]) (bool, error) {
		l, err := p.keeper.Licenses.Get(ctx, key.K3())
		if err != nil {
			return true, err
		}
		licenses = append(licenses, l)
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return licenses, nil
}

// LicensesByHolderAndType returns all licenses of a given type held by the given holder.
func (p Precompile) LicensesByHolderAndType(ctx sdk.Context, method *abi.Method, args []interface{}) ([]byte, error) {
	if err := argCount(args, 2); err != nil {
		return nil, err
	}
	holderHex, err := argToAddress(args[0], "holder")
	if err != nil {
		return nil, err
	}
	typeID, err := argToString(args[1], "typeId")
	if err != nil {
		return nil, err
	}
	holder, err := hexToBech32(p.addrCdc, holderHex)
	if err != nil {
		return nil, err
	}

	licenses, err := p.activeLicensesForHolder(ctx, collections.NewSuperPrefixedTripleRange[string, string, uint64](holder, typeID))
	if err != nil {
		return nil, err
	}

	out, err := licensesToOutputs(licenses)
	if err != nil {
		return nil, err
	}
	return method.Outputs.Pack(out)
}
