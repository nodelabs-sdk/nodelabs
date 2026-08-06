package types

import (
	"fmt"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// DefaultParams returns the module's default parameters. Defaults are
// chain-neutral: deauthorize_fee is empty (charge disabled) because this
// module cannot name a chain's denom. Consuming chains seed their real values
// in an upgrade handler or genesis.
//
// Activation still fail-closes out of the box, but that now comes from the
// node type registry rather than a param: with no node types registered, no
// node type resolves and nothing can activate.
//
// The owner is likewise empty: this module cannot name an address that is
// meaningful to an arbitrary chain, and an unset owner fails closed — nothing
// can be granted until governance sets one.
func DefaultParams() Params {
	return Params{
		ActivationLimitMultiplier: 3,
		SpamLimitMultiplier:       9,
		MaxActivationKeys:         5,
		RecentKeyLimit:            10,
		RecentKeyWindow:           24 * time.Hour,
		StatusDailyLimit:          10,
		DeauthorizeFee:            sdk.NewCoins(),
		MaxGaslessGas:             300_000,
		MaxGaslessMsgs:            10,
		MaxGaslessTxBytes:         50_000,
	}
}

// Validate checks the parameter set is well-formed.
func (p Params) Validate() error {
	if p.ActivationLimitMultiplier == 0 {
		return fmt.Errorf("activation_limit_multiplier must be positive")
	}
	if p.SpamLimitMultiplier < p.ActivationLimitMultiplier {
		return fmt.Errorf("spam_limit_multiplier (%d) must not be below activation_limit_multiplier (%d)", p.SpamLimitMultiplier, p.ActivationLimitMultiplier)
	}
	if p.MaxActivationKeys == 0 {
		return fmt.Errorf("max_activation_keys must be positive")
	}
	if p.RecentKeyLimit == 0 {
		return fmt.Errorf("recent_key_limit must be positive")
	}
	if p.RecentKeyWindow <= 0 {
		return fmt.Errorf("recent_key_window must be positive, got %s", p.RecentKeyWindow)
	}
	if p.StatusDailyLimit == 0 {
		return fmt.Errorf("status_daily_limit must be positive")
	}
	if err := p.DeauthorizeFee.Validate(); err != nil {
		return fmt.Errorf("invalid deauthorize_fee: %w", err)
	}
	if p.MaxGaslessGas == 0 {
		return fmt.Errorf("max_gasless_gas must be positive")
	}
	if p.MaxGaslessMsgs == 0 {
		return fmt.Errorf("max_gasless_msgs must be positive")
	}
	if p.MaxGaslessTxBytes == 0 {
		return fmt.Errorf("max_gasless_tx_bytes must be positive")
	}
	// An empty owner is valid and means "not yet set"; a non-empty one must be
	// a real address.
	// Canonical, not merely decodable: the owner is compared by string
	// equality on every owner-gated message, so a non-canonical alias would be
	// an owner that nobody — including its own key holder — can match.
	if p.Owner != "" {
		if err := ValidateCanonicalAddress("owner", p.Owner); err != nil {
			return err
		}
	}
	return nil
}

// Spec returns the module's static access configuration. This module does not
// scope its actions, so it declares no scope validator and every grant is
// module-wide.
func Spec() accesstypes.Spec {
	return accesstypes.Spec{Actions: ValidActions}
}
