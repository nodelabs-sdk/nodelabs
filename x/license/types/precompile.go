package types

// PrecompileAddress is the default EVM address at which the licenses precompile
// is registered. The layout is:
//
//	0x | 6e6f64656c616273 | 00000000000000000000 | 0001
//	     ascii("nodelabs")   zero padding           slot id
//
// Putting the ASCII bytes of "nodelabs" in the high-order bytes pushes the
// address far above any plausible cosmos/evm upstream precompile (currently
// clustered in 0x0100, 0x0400, and 0x0800-0x0806), so silent collision with a
// future upstream release is effectively impossible. The trailing 0x0001 is a
// per-precompile slot id: future custom precompiles in this chain can take
// 0x0002, 0x0003, … while sharing the nodelabs prefix.
//
// The app wiring also guards against collision defensively: it panics at
// start-up if the EVM keeper's static precompile map already has an entry for
// this address. See app/app.go where licensePrecompile is added to
// staticPrecompiles.
//
// Operators that want to register the precompile at a different address can do
// so when wiring it into the EVM keeper; this value is only consulted by the
// default registration helper.
// The mixed case is the EIP-55 checksum, not decoration: Solidity rejects an
// address literal whose case does not match, so LicenseI.sol will not compile
// if this value is edited without recomputing it.
const PrecompileAddress = "0x6E6F64656c616273000000000000000000000001"
