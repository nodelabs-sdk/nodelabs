package types

import (
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// DefaultParams returns the module's default parameters. The owner is empty:
// this module cannot name an address that is meaningful to an arbitrary chain,
// and an unset owner fails closed — nothing can be granted until governance
// sets one.
func DefaultParams() Params {
	return Params{Owner: ""}
}

// Validate checks the parameter set is well-formed. An empty owner is valid
// and means "not yet set"; a non-empty one must be a real address.
func (p Params) Validate() error {
	if p.Owner == "" {
		return nil
	}
	// Canonical, not merely decodable: the owner is compared by string
	// equality on every owner-gated message, so a non-canonical alias would be
	// an owner that nobody — including its own key holder — can match.
	return accesstypes.ValidateCanonicalAddress("owner", p.Owner)
}

// Spec returns the module's static access configuration. scopeExists resolves
// a license type id; the keeper supplies it, since only the keeper can read
// license state.
func Spec(scopeExists accesstypes.ScopeExistsFn) accesstypes.Spec {
	return accesstypes.Spec{
		Actions:     ValidActions,
		ScopeExists: scopeExists,
		// "type.create" authorizes creating the very license types the other
		// actions are scoped to, so it has no existing id to name.
		Unscoped: UnscopedActions,
	}
}
