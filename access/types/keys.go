package types

import "fmt"

// MaxGrants bounds the per-message slice lengths for grant operations: the
// top-level entry list of a grant or revoke message, and the inner Scopes
// slice within each entry.
const MaxGrants = 100

// ValidateAction validates an action name: non-empty, lowercase alphanumeric
// plus '.', '_' and '-'. The character set deliberately excludes the ',' and
// ':' delimiters used at the CLI boundary.
//
// Scopes are opaque to this package and carry no such restriction.
func ValidateAction(s string) error {
	if s == "" {
		return fmt.Errorf("action must not be empty")
	}
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return fmt.Errorf("invalid action %q: only lowercase letters, digits, '.', '_' and '-' are allowed", s)
		}
	}
	return nil
}
