package types

import "fmt"

// ValidateGrants performs stateless validation of a module's exported grant
// list: every grantee a valid address, every action a well-formed name, and no
// duplicate (grantee, action, scope) keys.
//
// The checks that need the module's spec — the action being in the vocabulary,
// the scope resolving — happen in Grants.Init, which runs after the module has
// written the state those scopes refer to.
func ValidateGrants(grants []Grant) error {
	keys := make(map[string]struct{}, len(grants))
	for i, g := range grants {
		// Canonical, not merely decodable: the grantee is a state key, and the
		// duplicate check below is an exact-string comparison, so two
		// encodings of one account would import as two distinct grants.
		if err := ValidateCanonicalAddress("grantee", g.Grantee); err != nil {
			return fmt.Errorf("grant %d: %w", i, err)
		}
		if err := ValidateAction(g.Action); err != nil {
			return fmt.Errorf("grant %d: %w", i, err)
		}

		key := fmt.Sprintf("%s/%s/%s", g.Grantee, g.Action, g.Scope)
		if _, dup := keys[key]; dup {
			return fmt.Errorf("duplicate grant (grantee=%s, action=%s, scope=%s)", g.Grantee, g.Action, g.Scope)
		}
		keys[key] = struct{}{}
	}
	return nil
}
