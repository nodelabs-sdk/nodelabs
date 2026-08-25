package app

import (
	"github.com/cosmos/cosmos-sdk/baseapp"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ baseapp.ProposalTxVerifier = &NoCheckProposalTxVerifier{}

// NoCheckProposalTxVerifier skips re-running the ante handlers at proposal
// time. With the Krakatoa mempool every tx in a proposal has already been
// validated on insert/recheck, so the default PrepareProposalVerifyTx (which
// runs the tx in checktx mode) is redundant work; only encoding is needed.
//
// Vendored from cosmos/evm's evmd (nested module we don't depend on).
type NoCheckProposalTxVerifier struct {
	*baseapp.BaseApp
}

func NewNoCheckProposalTxVerifier(b *baseapp.BaseApp) *NoCheckProposalTxVerifier {
	return &NoCheckProposalTxVerifier{BaseApp: b}
}

// PrepareProposalVerifyTx only verifies that the tx can be encoded to bytes;
// validity is guaranteed by the mempool's own CheckTx/Recheck lifecycle.
func (txv *NoCheckProposalTxVerifier) PrepareProposalVerifyTx(tx sdk.Tx) ([]byte, error) {
	return txv.TxEncode(tx)
}
