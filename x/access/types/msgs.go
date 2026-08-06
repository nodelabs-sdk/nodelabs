package types

import "fmt"

// ValidateGrantEntries performs the stateless checks a grant message's
// ValidateBasic owes its entry list: non-empty, within MaxGrants, and every
// action a well-formed name. Whether an action is in the module's vocabulary
// and whether its scopes resolve are stateful checks the grant store makes at
// handling time.
func ValidateGrantEntries(entries []ActionScopes) error {
	if len(entries) == 0 {
		return fmt.Errorf("grants must not be empty")
	}
	if len(entries) > MaxGrants {
		return fmt.Errorf("grants length %d exceeds max %d", len(entries), MaxGrants)
	}
	for i, e := range entries {
		if err := ValidateAction(e.Action); err != nil {
			return fmt.Errorf("grant %d: %w", i, err)
		}
		if len(e.Scopes) > MaxGrants {
			return fmt.Errorf("grant %d scopes length %d exceeds max %d", i, len(e.Scopes), MaxGrants)
		}
	}
	return nil
}

// ValidateRevokePairs is the ValidateGrantEntries counterpart for a revoke
// message's (action, scope) pair list.
func ValidateRevokePairs(pairs []ActionScope) error {
	if len(pairs) == 0 {
		return fmt.Errorf("actions must not be empty")
	}
	if len(pairs) > MaxGrants {
		return fmt.Errorf("actions length %d exceeds max %d", len(pairs), MaxGrants)
	}
	for i, p := range pairs {
		if err := ValidateAction(p.Action); err != nil {
			return fmt.Errorf("pair %d: %w", i, err)
		}
	}
	return nil
}
