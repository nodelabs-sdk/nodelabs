package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AccountKeeper is the x/auth keeper surface the license module consumes to
// create holder accounts on issuance, so a wallet holding only a license can
// sign its first transaction. Pubkeys are set lazily by the stock
// SetPubKeyDecorator, so accounts are created from the address only.
type AccountKeeper interface {
	GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
	HasAccount(ctx context.Context, addr sdk.AccAddress) bool
	NewAccountWithAddress(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
	SetAccount(ctx context.Context, acc sdk.AccountI)
}
