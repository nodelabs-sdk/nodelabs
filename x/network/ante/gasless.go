// Package ante provides the reusable gasless-transaction primitives for the
// x/network module: the allowlist predicate, a fee-checker wrapper, a
// decorator-skipping wrapper, hard resource caps, and the stateful admission
// decorator. Apps compose these into their cosmos ante chain; the EVM
// mono-handler path is untouched (gasless msgs never arrive as Ethereum
// txs).
package ante

import (
	"context"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// NewAllowlist builds the gasless msg-URL set from one or more msg URL
// lists (e.g. networktypes.GaslessMessages() plus a consuming chain's own).
func NewAllowlist(msgURLLists ...[]string) map[string]bool {
	allowlist := make(map[string]bool)
	for _, list := range msgURLLists {
		for _, url := range list {
			allowlist[url] = true
		}
	}
	return allowlist
}

// IsGaslessTx reports whether tx is admitted with a zero fee: every msg must
// be allowlisted and the declared fee must be exactly zero. Requiring both
// prevents smuggling paid msgs into free txs and vice versa; a tx mixing
// allowlisted and other msgs is a regular paid tx.
func IsGaslessTx(tx sdk.Tx, allowlist map[string]bool) bool {
	msgs := tx.GetMsgs()
	if len(msgs) == 0 {
		return false
	}
	for _, msg := range msgs {
		if !allowlist[sdk.MsgTypeURL(msg)] {
			return false
		}
	}
	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return false
	}
	return feeTx.GetFee().IsZero()
}

// NewGaslessTxFeeChecker wraps a TxFeeChecker: gasless txs clear with no fee
// and zero priority, everything else is delegated (e.g. to
// evmante.NewDynamicFeeChecker). The delegate must not be nil.
func NewGaslessTxFeeChecker(allowlist map[string]bool, delegate ante.TxFeeChecker) ante.TxFeeChecker {
	if delegate == nil {
		panic("gasless TxFeeChecker requires a non-nil delegate")
	}
	return func(ctx sdk.Context, tx sdk.Tx) (sdk.Coins, int64, error) {
		if IsGaslessTx(tx, allowlist) {
			return nil, 0, nil
		}
		return delegate(ctx, tx)
	}
}

// SkipForGaslessDecorator runs the wrapped decorator only for non-gasless
// txs. Used to bypass fee-floor decorators (e.g. MinGasPriceDecorator) that
// would reject a zero fee.
type SkipForGaslessDecorator struct {
	wrapped   sdk.AnteDecorator
	allowlist map[string]bool
}

func NewSkipForGaslessDecorator(wrapped sdk.AnteDecorator, allowlist map[string]bool) SkipForGaslessDecorator {
	return SkipForGaslessDecorator{wrapped: wrapped, allowlist: allowlist}
}

func (d SkipForGaslessDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if IsGaslessTx(tx, d.allowlist) {
		return next(ctx, tx, simulate)
	}
	return d.wrapped.AnteHandle(ctx, tx, simulate, next)
}

// ParamsGetter is the keeper surface the caps decorator reads the gasless
// resource caps from; satisfied by the network keeper.
type ParamsGetter interface {
	GetParams(ctx context.Context) (types.Params, error)
}

// GaslessCapsDecorator enforces hard resource caps on gasless txs: declared
// gas, msg count, and encoded size. It must run before
// SetUpContextDecorator hands out a gas meter — with the fee forced to zero
// nothing else bounds declared gas, and a single zero-fee tx declaring block
// max_gas would starve paid txs out of the proposer's reap at zero cost.
type GaslessCapsDecorator struct {
	pk        ParamsGetter
	allowlist map[string]bool
}

func NewGaslessCapsDecorator(pk ParamsGetter, allowlist map[string]bool) GaslessCapsDecorator {
	return GaslessCapsDecorator{pk: pk, allowlist: allowlist}
}

func (d GaslessCapsDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if !IsGaslessTx(tx, d.allowlist) {
		return next(ctx, tx, simulate)
	}

	params, err := d.pk.GetParams(ctx)
	if err != nil {
		return ctx, err
	}

	feeTx, ok := tx.(sdk.FeeTx)
	if !ok {
		return ctx, errorsmod.Wrapf(errortypes.ErrInvalidType, "invalid transaction type %T, expected sdk.FeeTx", tx)
	}
	if feeTx.GetGas() > params.MaxGaslessGas {
		return ctx, errorsmod.Wrapf(errortypes.ErrInvalidGasLimit, "gasless tx declares %d gas, cap is %d", feeTx.GetGas(), params.MaxGaslessGas)
	}
	if uint64(len(tx.GetMsgs())) > params.MaxGaslessMsgs {
		return ctx, errorsmod.Wrapf(errortypes.ErrTooManySignatures, "gasless tx carries %d msgs, cap is %d", len(tx.GetMsgs()), params.MaxGaslessMsgs)
	}
	// TxBytes can be empty under simulation; the size cap only applies to
	// real txs.
	if size := uint64(len(ctx.TxBytes())); size > params.MaxGaslessTxBytes {
		return ctx, errorsmod.Wrapf(errortypes.ErrTxTooLarge, "gasless tx is %d bytes, cap is %d", size, params.MaxGaslessTxBytes)
	}

	return next(ctx, tx, simulate)
}

// GaslessAdmission is implemented by module keepers that vet gasless msgs:
// the check verifies signer standing and rolls + increments the relevant
// daily counter. It runs in the ante chain, so its writes are committed even
// when the msg handler later fails (failing txs still burn quota), and
// over-limit spam is rejected at CheckTx before reaching a block. Handlers
// re-read the counters and never re-increment.
type GaslessAdmission interface {
	CheckAndConsumeGaslessQuota(ctx context.Context, msg sdk.Msg) error
}

// AdmissionRouter maps gasless msg type URLs to the keeper that admits
// them. Every allowlisted URL must be routed; admission fails closed on
// unrouted msgs.
type AdmissionRouter map[string]GaslessAdmission

// NewAdmissionRouter routes each URL in msgURLs to keeper.
func NewAdmissionRouter(keeper GaslessAdmission, msgURLs []string) AdmissionRouter {
	router := make(AdmissionRouter, len(msgURLs))
	for _, url := range msgURLs {
		router[url] = keeper
	}
	return router
}

// Merge adds other's routes and returns the router for chaining.
func (r AdmissionRouter) Merge(other AdmissionRouter) AdmissionRouter {
	for url, keeper := range other {
		r[url] = keeper
	}
	return r
}

// GaslessAdmissionDecorator dispatches every msg of a gasless tx to its
// module's admission check. Placed near the end of the ante chain so
// standing checks run against authenticated signers.
type GaslessAdmissionDecorator struct {
	allowlist map[string]bool
	router    AdmissionRouter
}

func NewGaslessAdmissionDecorator(allowlist map[string]bool, router AdmissionRouter) GaslessAdmissionDecorator {
	return GaslessAdmissionDecorator{allowlist: allowlist, router: router}
}

func (d GaslessAdmissionDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if !IsGaslessTx(tx, d.allowlist) {
		return next(ctx, tx, simulate)
	}

	for _, msg := range tx.GetMsgs() {
		url := sdk.MsgTypeURL(msg)
		admission, ok := d.router[url]
		if !ok {
			return ctx, errorsmod.Wrapf(errortypes.ErrUnknownRequest, "no gasless admission registered for %s", url)
		}
		if err := admission.CheckAndConsumeGaslessQuota(ctx, msg); err != nil {
			return ctx, err
		}
	}

	return next(ctx, tx, simulate)
}
