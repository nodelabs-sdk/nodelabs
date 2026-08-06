package license_test

import (
	"testing"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/x/license"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// TestDefaultGenesisJSONRoundTrip covers the `nodelabsd init` path: the JSON
// this module emits for a fresh chain must pass its own ValidateGenesis.
//
// It earns its own test because an empty GenesisState is deliberately invalid
// — next_license_id must be seeded to at least 1 so the first issued license
// gets id 1 rather than 0. If DefaultGenesis stopped emitting it, a new chain
// would fail to initialize.
func TestDefaultGenesisJSONRoundTrip(t *testing.T) {
	registry := codectypes.NewInterfaceRegistry()
	types.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	am := license.AppModuleBasic{}
	raw := am.DefaultGenesis(cdc)

	require.JSONEq(t, `{"license_types":[],"licenses":[],"next_license_id":"1","params":{"owner":""},"grants":[]}`, string(raw))
	require.NoError(t, am.ValidateGenesis(cdc, nil, raw))
}
