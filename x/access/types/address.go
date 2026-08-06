package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// ValidateCanonicalAddress checks that addr is a valid bech32 account address
// **in its canonical encoding**.
//
// Decoding successfully is not enough. BIP-173 permits an all-uppercase
// encoding of the same payload, and the bech32 library normalizes rather than
// rejects it (cosmos/btcutil bech32.Normalize lowercases before verifying the
// checksum), so "NODELABS1ABC..." and "nodelabs1abc..." decode to identical
// bytes. Signature verification compares those bytes, so both forms
// authenticate as the same account — but grants are keyed on the address
// *string*, so the two encodings are two distinct identities in the store.
//
// That difference is load-bearing for a grant store. A grant written under a
// non-canonical alias is exercisable by its holder — the signer authenticates
// on bytes — but invisible to a revoke or an audit query using the canonical
// form, both of which are exact-string operations. Revoking such a grant
// silently succeeds while removing nothing. Requiring the canonical form
// collapses each account to exactly one key, which is what makes revocation
// mean what it says.
//
// The error is returned unwrapped so each module can attach its own error type;
// see the wrappers in x/network/types.
func ValidateCanonicalAddress(field, addr string) error {
	decoded, err := sdk.AccAddressFromBech32(addr)
	if err != nil {
		return fmt.Errorf("invalid %s address: %w", field, err)
	}
	// Re-encoding always produces the canonical (lowercase, correct-prefix)
	// form, so a mismatch means the input was a non-canonical alias.
	if decoded.String() != addr {
		return fmt.Errorf("%s address %q is not in canonical form", field, addr)
	}
	return nil
}
