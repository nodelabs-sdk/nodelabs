package config

import (
	clienthelpers "cosmossdk.io/client/v2/helpers"
	serverconfig "github.com/cosmos/cosmos-sdk/server/config"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/crypto/hd"
	cosmosevmserverconfig "github.com/cosmos/evm/server/config"
	"github.com/cosmos/evm/utils"
)

const (
	// AppName is the name of the application binary.
	AppName = "nodelabsd"

	// DefaultNodeHomeDir is the default home directory name.
	DefaultNodeHomeDir = ".nodelabsd"

	// Bech32Prefix is the bech32 prefix used for account, validator and consensus
	// addresses on the nodelabs chain. Keep this in sync with BECH32_PREFIX in
	// scripts/testnet.sh.
	Bech32Prefix = "nodelabs"

	Bech32PrefixAccAddr  = Bech32Prefix
	Bech32PrefixAccPub   = Bech32Prefix + sdk.PrefixPublic
	Bech32PrefixValAddr  = Bech32Prefix + sdk.PrefixValidator + sdk.PrefixOperator
	Bech32PrefixValPub   = Bech32Prefix + sdk.PrefixValidator + sdk.PrefixOperator + sdk.PrefixPublic
	Bech32PrefixConsAddr = Bech32Prefix + sdk.PrefixValidator + sdk.PrefixConsensus
	Bech32PrefixConsPub  = Bech32Prefix + sdk.PrefixValidator + sdk.PrefixConsensus + sdk.PrefixPublic
)

// MustGetDefaultNodeHome returns the default node home directory.
func MustGetDefaultNodeHome() string {
	defaultNodeHome, err := clienthelpers.GetNodeHomeDirectory(DefaultNodeHomeDir)
	if err != nil {
		panic(err)
	}
	return defaultNodeHome
}

// SetBech32Prefixes installs nodelabs's bech32 prefixes on the global SDK config.
// Upstream's SetBech32Prefixes hard-codes "cosmos"; we override here so CLI
// tooling (keys add, query, tx) emits nodelabs-prefixed addresses.
func SetBech32Prefixes(config *sdk.Config) {
	config.SetBech32PrefixForAccount(Bech32PrefixAccAddr, Bech32PrefixAccPub)
	config.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
	config.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)
}

// SetBip44CoinType sets the global coin type to be used in hierarchical
// deterministic wallets.
//
// Vendored from cosmos/evm's evmd/config (the top-level evm/config package was
// removed in v0.7); evmd/config lives in a nested Go module we don't want to
// depend on.
func SetBip44CoinType(config *sdk.Config) {
	config.SetCoinType(hd.Bip44CoinType)
	config.SetPurpose(sdk.Purpose)               // Shared
	config.SetFullFundraiserPath(hd.BIP44HDPath) //nolint:staticcheck
}

// InitAppConfig helps to override default appConfig template and configs.
//
// Vendored from cosmos/evm's evmd/config for the same reason as
// SetBip44CoinType.
func InitAppConfig(denom string, evmChainID uint64) (string, interface{}) {
	srvCfg := serverconfig.DefaultConfig()
	// The SDK's default minimum gas price is set to "" (empty value) inside
	// app.toml, which would make the node halt on startup if left untouched.
	srvCfg.MinGasPrices = "0" + denom

	evmCfg := cosmosevmserverconfig.DefaultEVMConfig()
	evmCfg.EVMChainID = evmChainID

	customAppConfig := EVMAppConfig{
		Config:  *srvCfg,
		EVM:     *evmCfg,
		JSONRPC: *cosmosevmserverconfig.DefaultJSONRPCConfig(),
		TLS:     *cosmosevmserverconfig.DefaultTLSConfig(),
	}

	return EVMAppTemplate, customAppConfig
}

// EVMAppConfig is the app.toml layout: the SDK server config extended with the
// cosmos/evm EVM, JSON-RPC and TLS sections.
type EVMAppConfig struct {
	serverconfig.Config

	EVM     cosmosevmserverconfig.EVMConfig
	JSONRPC cosmosevmserverconfig.JSONRPCConfig
	TLS     cosmosevmserverconfig.TLSConfig
}

// EVMAppTemplate is the app.toml template matching EVMAppConfig.
const EVMAppTemplate = serverconfig.DefaultConfigTemplate + cosmosevmserverconfig.DefaultEVMConfigTemplate

// GetChainIDFromHome returns the chain id from the client config in the given
// home directory. (Moved upstream from evm/config to evm/utils in v0.7.)
var GetChainIDFromHome = utils.GetChainIDFromHome
