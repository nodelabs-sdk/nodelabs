package licenseprecompile

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/core/address"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	evmaddress "github.com/cosmos/evm/encoding/address"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// addrCodec returns an evm-bech32 address codec bound to the chain's account prefix.
func addrCodec(t testing.TB) address.Codec {
	t.Helper()
	return evmaddress.NewEvmCodec(sdk.GetConfig().GetBech32AccountAddrPrefix())
}

// TestHexBech32Roundtrip asserts hex→bech32→hex is identity for valid EVM addresses.
func TestHexBech32Roundtrip(t *testing.T) {
	cdc := addrCodec(t)
	hex := common.HexToAddress("0x1234567890123456789012345678901234567890")

	bech, err := hexToBech32(cdc, hex)
	require.NoError(t, err)
	require.NotEmpty(t, bech)

	got, err := bech32ToHex(bech)
	require.NoError(t, err)
	require.Equal(t, hex, got)
}

// TestBech32ToHexEmpty: empty string is the legitimate "unset" case (zero, nil).
func TestBech32ToHexEmpty(t *testing.T) {
	hex, err := bech32ToHex("")
	require.NoError(t, err)
	require.Equal(t, common.Address{}, hex)
}

// TestBech32ToHexInvalid: non-empty malformed input surfaces an error so
// callers can flag state corruption instead of silently emitting a zero
// address.
func TestBech32ToHexInvalid(t *testing.T) {
	_, err := bech32ToHex("not-a-bech32-string")
	require.Error(t, err)
}

// TestBigIntFromCosmosInt converts cosmos math.Ints, including the uninitialised
// zero value, into safe *big.Ints.
func TestBigIntFromCosmosInt(t *testing.T) {
	require.Equal(t, big.NewInt(0), bigIntFromCosmosInt(math.Int{}))
	require.Equal(t, big.NewInt(0), bigIntFromCosmosInt(math.ZeroInt()))
	require.Equal(t, big.NewInt(42), bigIntFromCosmosInt(math.NewInt(42)))
}

// TestLicenseTypeToOutput maps every field; nil math.Ints must not panic.
// The struct holds no address, so unlike licenseToOutput the conversion is
// total and returns no error.
func TestLicenseTypeToOutput(t *testing.T) {
	lt := licensetypes.LicenseType{
		Id:            "type.a",
		Transferrable: true,
		MaxSupply:     math.NewInt(100),
		IssuedCount:   math.NewInt(7),
		ActiveCount:   math.NewInt(5),
		RevokedCount:  math.NewInt(2),
	}

	out := licenseTypeToOutput(lt)
	require.Equal(t, "type.a", out.Id)
	require.True(t, out.Transferrable)
	require.Equal(t, big.NewInt(100), out.MaxSupply)
	require.Equal(t, big.NewInt(7), out.IssuedCount)
	require.Equal(t, big.NewInt(5), out.ActiveCount)
	require.Equal(t, big.NewInt(2), out.RevokedCount)

	// Nil counters convert to zero rather than panicking.
	out = licenseTypeToOutput(licensetypes.LicenseType{Id: "bare"})
	require.Equal(t, "bare", out.Id)
	require.Equal(t, big.NewInt(0), out.MaxSupply)
	require.Equal(t, big.NewInt(0), out.IssuedCount)
	require.Equal(t, big.NewInt(0), out.ActiveCount)
	require.Equal(t, big.NewInt(0), out.RevokedCount)
}

// TestLicenseToOutput converts an SDK License into its EVM-shaped counterpart,
// notably mapping bech32 holders to address types.
func TestLicenseToOutput(t *testing.T) {
	cdc := addrCodec(t)
	hex := common.HexToAddress("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	bech, err := hexToBech32(cdc, hex)
	require.NoError(t, err)

	l := licensetypes.License{
		Id:        42,
		Type:      "type.a",
		Holder:    bech,
		StartDate: "2025-01-01",
		EndDate:   "2026-01-01",
		Status:    licensetypes.StatusActive,
	}

	out, err := licenseToOutput(l)
	require.NoError(t, err)
	require.Equal(t, uint64(42), out.Id)
	require.Equal(t, "type.a", out.TypeId)
	require.Equal(t, hex, out.Holder)
	require.Equal(t, "2025-01-01", out.StartDate)
	require.Equal(t, "2026-01-01", out.EndDate)
	require.Equal(t, "active", out.Status)
}

// TestArgCount accepts the exact count and rejects everything else.
func TestArgCount(t *testing.T) {
	require.NoError(t, argCount([]interface{}{}, 0))
	require.NoError(t, argCount([]interface{}{1, 2, 3}, 3))
	require.Error(t, argCount([]interface{}{1}, 2))
	require.Error(t, argCount([]interface{}{1, 2, 3}, 2))
}
