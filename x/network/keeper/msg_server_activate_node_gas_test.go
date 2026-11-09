package keeper_test

import (
	"testing"
	"time"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/x/network/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// activationGas runs MsgActivateNode's ante admission and handler on a fresh
// gas meter and returns the gas they consumed together.
func activationGas(t testing.TB, f *keepertest.NetworkFixture, ms types.MsgServer, ctx sdk.Context, key, operator string) uint64 {
	t.Helper()
	ctx = ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	msg := &types.MsgActivateNode{ActivationAddress: key, Operator: operator, NodeAddress: sample.AccAddress(), NodeType: f.NodeType}
	require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(ctx, msg))
	_, err := ms.ActivateNode(ctx, msg)
	require.NoError(t, err)
	return ctx.GasMeter().GasConsumed()
}

// requireFlat asserts gas stays within 1% of a reference across a run.
func requireFlat(t *testing.T, phase string, ref uint64, gas map[int]uint64) {
	t.Helper()
	for i, g := range gas {
		require.InEpsilon(t, float64(ref), float64(g), 0.01, "%s: activation %d used %d gas, reference %d", phase, i, g, ref)
	}
}

// TestActivateNodeGasIndependentOfFleetSize guards against activation checks
// that walk an operator's fleet or licenses. Such walks made each activation
// cost ~490 more gas than the one before, crossing the 300k gasless cap at
// ~416 nodes. Admission plus handler must now cost the same at any fleet size,
// on both the normal path and the replacement path.
//
// Addresses carry the test default "cosmos" bech32 prefix, two characters
// shorter than the chain's "nodelabs"; that shifts absolute figures by a few
// gas per key read, not the shape this test pins.
func TestActivateNodeGasIndependentOfFleetSize(t *testing.T) {
	f, ms := setup(t)
	op := sample.AccAddress()
	const licenses = 1000
	f.IssueLicenses(t, op, licenses)
	key := authorizeKey(t, f, ms, f.Ctx, op)

	// Normal path: Total < 3 x licenses, decided without the recent window.
	normal := make(map[int]uint64)
	for i := 1; i <= licenses; i++ {
		g := activationGas(t, f, ms, f.Ctx, key, op)
		if i == 1 || i == 2 || i == 416 || i%250 == 0 {
			normal[i] = g
		}
	}
	requireFlat(t, "normal path", normal[1], normal)

	// Fill the lifetime allowance (Total = 3 x licenses), then come back two
	// days later when the recent window has drained: every activation now
	// takes the replacement path (Total >= limit, recent < limit), which also
	// reads the window.
	for i := licenses + 1; i <= 3*licenses; i++ {
		activationGas(t, f, ms, f.Ctx, key, op)
	}
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(48 * time.Hour))
	replacement := make(map[int]uint64)
	for i := 1; i <= licenses; i++ {
		g := activationGas(t, f, ms, later, key, op)
		if i == 1 || i == 2 || i%250 == 0 {
			replacement[i] = g
		}
	}
	requireFlat(t, "replacement path", replacement[1], replacement)

	counts, err := f.Keeper.GetOperatorNodeCounts(f.Ctx, op, f.NodeType)
	require.NoError(t, err)
	require.Equal(t, uint64(4*licenses), counts.Total)

	t.Logf("normal path: #1=%d #2=%d #416=%d #1000=%d", normal[1], normal[2], normal[416], normal[1000])
	t.Logf("replacement path (fleet 3001-4000): #1=%d #1000=%d", replacement[1], replacement[1000])
}
