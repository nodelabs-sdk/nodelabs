package sample

import (
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AccAddress returns a random bech32 account address.
func AccAddress() string {
	pk := secp256k1.GenPrivKey().PubKey()
	addr := sdk.AccAddress(pk.Address())
	return addr.String()
}
