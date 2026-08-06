package ante_test

import (
	"context"
	"errors"
	"testing"

	protov2 "google.golang.org/protobuf/proto"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	networkante "github.com/nodelabs-sdk/nodelabs/x/network/ante"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// fakeTx is a minimal sdk.FeeTx for exercising the primitives.
type fakeTx struct {
	msgs []sdk.Msg
	fee  sdk.Coins
	gas  uint64
}

func (t fakeTx) GetMsgs() []sdk.Msg                    { return t.msgs }
func (t fakeTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }
func (t fakeTx) GetGas() uint64                        { return t.gas }
func (t fakeTx) GetFee() sdk.Coins                     { return t.fee }
func (t fakeTx) FeePayer() []byte                      { return nil }
func (t fakeTx) FeeGranter() []byte                    { return nil }

var (
	gaslessMsg = &types.MsgUpdateNodeStatus{}
	paidMsg    = &types.MsgDeauthorizeActivationKey{}
	allowlist  = networkante.NewAllowlist(types.GaslessMessages())
)

func passthrough(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }

func TestIsGaslessTx(t *testing.T) {
	fee := sdk.NewCoins(sdk.NewInt64Coin("stake", 1))

	tests := []struct {
		name string
		tx   fakeTx
		want bool
	}{
		{name: "allowlisted msgs, zero fee", tx: fakeTx{msgs: []sdk.Msg{gaslessMsg, gaslessMsg}}, want: true},
		{name: "allowlisted msg, non-zero fee", tx: fakeTx{msgs: []sdk.Msg{gaslessMsg}, fee: fee}, want: false},
		{name: "non-allowlisted msg, zero fee", tx: fakeTx{msgs: []sdk.Msg{paidMsg}}, want: false},
		{name: "mixed msgs, zero fee", tx: fakeTx{msgs: []sdk.Msg{gaslessMsg, paidMsg}}, want: false},
		{name: "no msgs", tx: fakeTx{}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, networkante.IsGaslessTx(tc.tx, allowlist))
		})
	}
}

func TestGaslessTxFeeChecker(t *testing.T) {
	delegated := errors.New("delegated")
	checker := networkante.NewGaslessTxFeeChecker(allowlist, func(sdk.Context, sdk.Tx) (sdk.Coins, int64, error) {
		return nil, 0, delegated
	})

	// Gasless txs clear with no fee and zero priority.
	fee, priority, err := checker(sdk.Context{}, fakeTx{msgs: []sdk.Msg{gaslessMsg}})
	require.NoError(t, err)
	require.True(t, fee.IsZero())
	require.Zero(t, priority)

	// Everything else is delegated.
	_, _, err = checker(sdk.Context{}, fakeTx{msgs: []sdk.Msg{paidMsg}})
	require.ErrorIs(t, err, delegated)

	// A zero-fee tx with a paid msg is not gasless — it is delegated (and
	// will fail the real fee floor).
	_, _, err = checker(sdk.Context{}, fakeTx{msgs: []sdk.Msg{gaslessMsg, paidMsg}})
	require.ErrorIs(t, err, delegated)

	require.Panics(t, func() { networkante.NewGaslessTxFeeChecker(allowlist, nil) })
}

type failingDecorator struct{}

func (failingDecorator) AnteHandle(ctx sdk.Context, _ sdk.Tx, _ bool, _ sdk.AnteHandler) (sdk.Context, error) {
	return ctx, errors.New("wrapped decorator ran")
}

func TestSkipForGaslessDecorator(t *testing.T) {
	d := networkante.NewSkipForGaslessDecorator(failingDecorator{}, allowlist)

	// Gasless txs skip the wrapped decorator.
	_, err := d.AnteHandle(sdk.Context{}, fakeTx{msgs: []sdk.Msg{gaslessMsg}}, false, passthrough)
	require.NoError(t, err)

	// Paid txs run it.
	_, err = d.AnteHandle(sdk.Context{}, fakeTx{msgs: []sdk.Msg{paidMsg}}, false, passthrough)
	require.ErrorContains(t, err, "wrapped decorator ran")
}

func TestGaslessCapsDecorator(t *testing.T) {
	f := keepertest.NewNetworkFixture(t)
	d := networkante.NewGaslessCapsDecorator(f.Keeper, allowlist)

	params, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)

	// Within caps.
	_, err = d.AnteHandle(f.Ctx, fakeTx{msgs: []sdk.Msg{gaslessMsg}, gas: params.MaxGaslessGas}, false, passthrough)
	require.NoError(t, err)

	// Declared gas above the cap.
	_, err = d.AnteHandle(f.Ctx, fakeTx{msgs: []sdk.Msg{gaslessMsg}, gas: params.MaxGaslessGas + 1}, false, passthrough)
	require.ErrorContains(t, err, "gas")

	// Msg count above the cap.
	msgs := make([]sdk.Msg, params.MaxGaslessMsgs+1)
	for i := range msgs {
		msgs[i] = gaslessMsg
	}
	_, err = d.AnteHandle(f.Ctx, fakeTx{msgs: msgs}, false, passthrough)
	require.ErrorContains(t, err, "msgs")

	// Tx bytes above the cap.
	bigTx := f.Ctx.WithTxBytes(make([]byte, params.MaxGaslessTxBytes+1))
	_, err = d.AnteHandle(bigTx, fakeTx{msgs: []sdk.Msg{gaslessMsg}}, false, passthrough)
	require.ErrorContains(t, err, "bytes")

	// A paid tx sails past all caps.
	_, err = d.AnteHandle(bigTx, fakeTx{msgs: []sdk.Msg{paidMsg}, gas: params.MaxGaslessGas * 100}, false, passthrough)
	require.NoError(t, err)
}

type recordingAdmission struct {
	seen []string
	err  error
}

func (r *recordingAdmission) CheckAndConsumeGaslessQuota(_ context.Context, msg sdk.Msg) error {
	r.seen = append(r.seen, sdk.MsgTypeURL(msg))
	return r.err
}

func TestGaslessAdmissionDecorator(t *testing.T) {
	adm := &recordingAdmission{}
	router := networkante.NewAdmissionRouter(adm, types.GaslessMessages())
	d := networkante.NewGaslessAdmissionDecorator(allowlist, router)

	// Every msg of a gasless tx is dispatched.
	_, err := d.AnteHandle(sdk.Context{}, fakeTx{msgs: []sdk.Msg{gaslessMsg, gaslessMsg}}, false, passthrough)
	require.NoError(t, err)
	require.Len(t, adm.seen, 2)

	// Admission failures reject the tx.
	adm.err = errors.New("quota exceeded")
	_, err = d.AnteHandle(sdk.Context{}, fakeTx{msgs: []sdk.Msg{gaslessMsg}}, false, passthrough)
	require.ErrorContains(t, err, "quota exceeded")

	// Paid txs are not dispatched.
	adm.seen = nil
	_, err = d.AnteHandle(sdk.Context{}, fakeTx{msgs: []sdk.Msg{paidMsg}}, false, passthrough)
	require.NoError(t, err)
	require.Empty(t, adm.seen)

	// An allowlisted msg with no route fails closed.
	bare := networkante.NewGaslessAdmissionDecorator(allowlist, networkante.AdmissionRouter{})
	_, err = bare.AnteHandle(sdk.Context{}, fakeTx{msgs: []sdk.Msg{gaslessMsg}}, false, passthrough)
	require.ErrorContains(t, err, "no gasless admission registered")
}
