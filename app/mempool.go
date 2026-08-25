package app

import (
	evmmempool "github.com/cosmos/evm/mempool"
	"github.com/cosmos/evm/server"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// configureEVMMempool wires the Krakatoa application-side EVM mempool and its
// ABCI handlers from viper configuration. Mirrors the reference evmd wiring.
//
// The JSON-RPC server in cosmos/evm v0.7+ requires the app mempool (it
// type-asserts GetMempool() to backend.Mempool at startup), so nodes serving
// JSON-RPC cannot opt out. Opting out entirely (stock CometBFT mempool, no
// JSON-RPC) is still possible with mempool.max-txs=-1.
//
// NOTE: must be called after app.SetAnteHandler — ResolveMempoolConfig
// captures the ante handler for the recheckers, and a nil handler panics on
// the first RecheckTx.
func (app *NodelabsApp) configureEVMMempool(appOpts servertypes.AppOptions, logger log.Logger) error {
	if evmtypes.GetChainConfig() == nil {
		logger.Debug("evm chain config is not set, skipping mempool configuration")
		return nil
	}

	var (
		mpConfig = server.ResolveMempoolConfig(app.GetAnteHandler(), appOpts, logger)

		txEncoder = evmmempool.NewTxEncoder(app.txConfig)
		// evmRechecker and cosmosRechecker must be distinct instances: they
		// feed different subpools and keep independent promotion/demotion
		// state. Sharing one compiles but corrupts cross-pool bookkeeping.
		evmRechecker    = evmmempool.NewTxRechecker(mpConfig.AnteHandler, txEncoder)
		cosmosRechecker = evmmempool.NewTxRechecker(mpConfig.AnteHandler, txEncoder)
		cosmosPoolMaxTx = server.GetCosmosPoolMaxTx(appOpts, logger)
		checkTxTimeout  = server.GetMempoolCheckTxTimeout(appOpts, logger)
	)

	if cosmosPoolMaxTx < 0 {
		logger.Debug("evm mempool is disabled, skipping configuration")
		return nil
	}

	if err := server.ValidateReapBounds(appOpts, mpConfig.BlockGasLimit); err != nil {
		return err
	}

	mempool := evmmempool.NewMempool(
		app.CreateQueryContext,
		logger,
		app.EVMKeeper,
		app.FeeMarketKeeper,
		app.txConfig,
		evmRechecker,
		cosmosRechecker,
		mpConfig,
		cosmosPoolMaxTx,
	)

	app.EVMMempool = mempool

	// create ABCI handlers
	proposalHandler := baseapp.NewDefaultProposalHandler(mempool, NewNoCheckProposalTxVerifier(app.BaseApp))

	insertTxHandler := mempool.NewInsertTxHandler(app.TxDecode)
	reapTxsHandler := mempool.NewReapTxsHandler()
	checkTxHandler := mempool.NewCheckTxHandler(app.TxDecode, checkTxTimeout)

	// set handlers and the mempool
	app.SetPrepareProposal(proposalHandler.PrepareProposalHandler())
	app.SetProcessProposal(proposalHandler.ProcessProposalHandler())
	app.SetInsertTxHandler(insertTxHandler)
	app.SetReapTxsHandler(reapTxsHandler)
	app.SetCheckTxHandler(checkTxHandler)

	app.SetMempool(mempool)

	app.SetPrepareCheckStater(func(_ sdk.Context) {
		if !mempool.HasEventBus() {
			mempool.NotifyNewBlock()
		}
	})

	return nil
}
