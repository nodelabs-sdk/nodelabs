package types

import (
	"context"
	"fmt"
	"sort"
)

// ScopeExistsFn reports whether a scope identifier refers to an existing
// resource in the owning module (e.g. a license type id). It is consulted at
// grant time and during genesis import.
type ScopeExistsFn func(ctx context.Context, scope string) (bool, error)

// Spec is the static configuration a module supplies when it builds its grant
// store: the action vocabulary it recognises and, optionally, a scope
// validator. It is compile-time configuration rather than state — every node
// builds the same spec, so consulting it is deterministic.
type Spec struct {
	// Actions is the full set of action names the module uses.
	Actions []string

	// ScopeExists validates scope identifiers at grant time. When nil, scopes
	// are unconstrained opaque strings and may be empty (module-wide grants).
	// When set, scopes must be non-empty and pass the check — except for the
	// actions named in Unscoped.
	ScopeExists ScopeExistsFn

	// Unscoped names the actions that are module-wide even though the module
	// otherwise scopes. Grants for these must carry the empty scope and
	// ScopeExists is not consulted for them.
	//
	// It exists for actions that have no resource to point at — the right to
	// create the very resources the other actions are scoped to cannot name
	// one, since it does not exist yet. Without this, such an action is
	// inexpressible in a scoped module.
	//
	// Only meaningful alongside a non-nil ScopeExists: a module with no scope
	// validator is already module-wide throughout.
	Unscoped []string
}

// Validate checks the spec is well-formed: at least one action, each action a
// valid name, no duplicates, and every unscoped action a member of a
// vocabulary that the module actually scopes.
func (s Spec) Validate() error {
	if len(s.Actions) == 0 {
		return fmt.Errorf("spec must declare at least one action")
	}
	seen := make(map[string]struct{}, len(s.Actions))
	for _, a := range s.Actions {
		if err := ValidateAction(a); err != nil {
			return err
		}
		if _, dup := seen[a]; dup {
			return fmt.Errorf("duplicate action %q", a)
		}
		seen[a] = struct{}{}
	}

	if len(s.Unscoped) == 0 {
		return nil
	}
	// Both remaining checks reject specs where Unscoped changes nothing, on
	// the grounds that declaring it means the author expected it to.
	if s.ScopeExists == nil {
		return fmt.Errorf("unscoped actions declared without a scope validator: every action here is already module-wide")
	}
	unscoped := make(map[string]struct{}, len(s.Unscoped))
	for _, a := range s.Unscoped {
		if _, ok := seen[a]; !ok {
			return fmt.Errorf("unscoped action %q is not in the vocabulary", a)
		}
		if _, dup := unscoped[a]; dup {
			return fmt.Errorf("duplicate unscoped action %q", a)
		}
		unscoped[a] = struct{}{}
	}
	if len(unscoped) == len(seen) {
		return fmt.Errorf("all %d actions are declared unscoped: drop ScopeExists instead", len(seen))
	}
	return nil
}

// HasAction reports whether a is in the spec's vocabulary.
func (s Spec) HasAction(a string) bool {
	for _, sa := range s.Actions {
		if sa == a {
			return true
		}
	}
	return false
}

// IsUnscoped reports whether a is granted module-wide despite the module
// carrying a scope validator. Always false when Unscoped is empty, which
// includes every module that does not scope at all.
func (s Spec) IsUnscoped(a string) bool {
	for _, ua := range s.Unscoped {
		if ua == a {
			return true
		}
	}
	return false
}

// SortedActions returns the vocabulary in ascending order without mutating the
// spec.
func (s Spec) SortedActions() []string {
	out := make([]string, len(s.Actions))
	copy(out, s.Actions)
	sort.Strings(out)
	return out
}

// SortedUnscoped returns the module-wide actions in ascending order without
// mutating the spec.
func (s Spec) SortedUnscoped() []string {
	out := make([]string, len(s.Unscoped))
	copy(out, s.Unscoped)
	sort.Strings(out)
	return out
}
