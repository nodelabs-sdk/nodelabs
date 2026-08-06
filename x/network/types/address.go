package types

import (
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// ValidateCanonicalAddress checks that addr is a valid bech32 account address
// in its canonical encoding, and reports a failure as this module's
// ErrInvalidAddress.
//
// The check itself lives in x/access/types so that every module keying state on
// an address string shares one definition; see the rationale there. This module
// needs it for more than grants: node addresses and activation addresses are
// single-use, and it is the presence of their record that burns them
// (Nodes.Has / ActivationKeys.Has). Accepting a non-canonical encoding would
// let a caller re-activate a deactivated node or re-authorize a deauthorized
// activation address simply by changing the case.
func ValidateCanonicalAddress(field, addr string) error {
	if err := accesstypes.ValidateCanonicalAddress(field, addr); err != nil {
		return ErrInvalidAddress.Wrap(err.Error())
	}
	return nil
}
