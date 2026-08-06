package config

import (
	evmconfig "github.com/cosmos/evm/config"

	clienthelpers "cosmossdk.io/client/v2/helpers"
	sdk "github.com/cosmos/cosmos-sdk/types"
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
// Upstream's evmconfig.SetBech32Prefixes hard-codes "cosmos"; we override here so
// CLI tooling (keys add, query, tx) emits nodelabs-prefixed addresses.
func SetBech32Prefixes(config *sdk.Config) {
	config.SetBech32PrefixForAccount(Bech32PrefixAccAddr, Bech32PrefixAccPub)
	config.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
	config.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)
}

// SetBip44CoinType wraps the cosmos evm config SetBip44CoinType.
var SetBip44CoinType = evmconfig.SetBip44CoinType

// InitAppConfig wraps the cosmos evm config InitAppConfig.
var InitAppConfig = evmconfig.InitAppConfig

// GetChainIDFromHome wraps the cosmos evm config GetChainIDFromHome.
var GetChainIDFromHome = evmconfig.GetChainIDFromHome
