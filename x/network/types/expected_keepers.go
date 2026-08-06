package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// LicenseKeeper is the x/license keeper surface the network module consumes.
type LicenseKeeper interface {
	// CountActiveLicenses returns the number of active licenses holder holds
	// across the given license types. When stopAt is non-zero the walk stops
	// as soon as the count reaches it, so gas is bounded by the check being
	// made rather than by holdings; stopAt zero counts everything.
	CountActiveLicenses(ctx context.Context, holder string, licenseTypes []string, stopAt uint64) (uint64, error)

	// HasLicenseType reports whether license type id exists. Existence is all
	// this module needs in order to bind a node type to it; taking the whole
	// record would couple this package to the license type's shape.
	HasLicenseType(ctx context.Context, id string) (bool, error)
}

// AccountKeeper is the x/auth keeper surface the network module consumes to
// create accounts for operator wallets, activation addresses, and node
// addresses. Pubkeys are set lazily by the stock SetPubKeyDecorator on each
// account's first signed tx, so accounts are created from the address only.
type AccountKeeper interface {
	HasAccount(ctx context.Context, addr sdk.AccAddress) bool
	NewAccountWithAddress(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
	SetAccount(ctx context.Context, acc sdk.AccountI)
}

// BankKeeper is the x/bank keeper surface the network module consumes to
// charge the deauthorize fee.
type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
}
