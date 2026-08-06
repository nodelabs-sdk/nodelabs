package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// GrantKey is the flat state key of a grant: (grantee, action, scope). The
// order makes an access check a point-read and a grantee's holdings a
// prefix walk.
type GrantKey = collections.Triple[string, string, string]

// Grants is a module's grant store, embedded in that module's keeper and
// bound to a prefix in that module's KVStore. Ownership is not held here: the
// owner is a module parameter, and the owner check belongs to the handler that
// reads it.
//
// The zero value is not usable; build one with NewGrants.
type Grants struct {
	// module names the owning module. It appears in error messages and in the
	// emitted events, and is not part of any state key — the store is already
	// the module boundary.
	module string

	spec types.Spec
	set  collections.KeySet[GrantKey]
}

// NewGrants builds a module's grant store. spec is static configuration, so an
// invalid one is a wiring bug and panics at construction rather than failing a
// transaction later.
//
// Modules that scope their actions supply spec.ScopeExists as a closure over
// the collection holding the scoped resource — not over the keeper, which is
// still being built at this point.
func NewGrants(sb *collections.SchemaBuilder, prefix collections.Prefix, name, module string, spec types.Spec) Grants {
	if err := spec.Validate(); err != nil {
		panic(fmt.Errorf("access spec for module %q: %w", module, err))
	}

	return Grants{
		module: module,
		spec:   spec,
		set: collections.NewKeySet(sb, prefix, name,
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey)),
	}
}

// Module returns the name of the module this store belongs to.
func (g Grants) Module() string { return g.module }

// Spec returns the module's static action vocabulary and scope rules.
func (g Grants) Spec() types.Spec { return g.spec }

// KeySet exposes the underlying collection so the module's query server can
// paginate over it. Prefer the helpers in query.go.
func (g Grants) KeySet() collections.KeySet[GrantKey] { return g.set }

// HasGrant reports whether grantee holds the (action, scope) grant. A missing
// grant returns (false, nil); a store error is surfaced so the caller can fail
// the transaction instead of silently denying the action.
func (g Grants) HasGrant(ctx context.Context, grantee, action, scope string) (bool, error) {
	return g.set.Has(ctx, collections.Join3(grantee, action, scope))
}

// Can is the yes/no convenience form of HasGrant: any underlying store error
// is treated as "no". Callers that must distinguish "missing" from "store
// failure" — which is every message handler — should use HasGrant.
func (g Grants) Can(ctx context.Context, grantee, action, scope string) bool {
	ok, _ := g.HasGrant(ctx, grantee, action, scope)
	return ok
}

// Set writes a single grant with no spec validation and no event. It is the
// raw escape hatch for test fixtures and migrations; handlers use GrantTo.
func (g Grants) Set(ctx context.Context, grantee, action, scope string) error {
	return g.set.Set(ctx, collections.Join3(grantee, action, scope))
}

// Remove deletes a single grant with no event, the inverse of Set. Removing a
// grant that is not present is not an error.
func (g Grants) Remove(ctx context.Context, grantee, action, scope string) error {
	return g.set.Remove(ctx, collections.Join3(grantee, action, scope))
}

// GrantTo merges entries into grantee's existing grants. Existing pairs are
// never removed and re-granting an existing pair is an idempotent overwrite;
// use RevokeFrom to remove pairs.
//
// Every (action, scope) pair is resolved and validated before anything is
// written, so a partially-invalid call grants nothing.
//
// On success it emits the grant event itself, so that every module reports the
// same thing for the same operation. ctx must be an SDK context.
func (g Grants) GrantTo(ctx context.Context, grantee string, entries []types.ActionScopes) error {
	if len(entries) > types.MaxGrants {
		return fmt.Errorf("grants length %d exceeds max %d", len(entries), types.MaxGrants)
	}

	type pair struct{ action, scope string }
	var pairs []pair
	for i, e := range entries {
		if len(e.Scopes) > types.MaxGrants {
			return fmt.Errorf("grant %d scopes length %d exceeds max %d", i, len(e.Scopes), types.MaxGrants)
		}
		// An empty scope list is the module-wide grant form; validatePair
		// rejects it when the module scopes that action.
		scopes := e.Scopes
		if len(scopes) == 0 {
			scopes = []string{""}
		}
		for _, scope := range scopes {
			if err := g.validatePair(ctx, e.Action, scope); err != nil {
				return fmt.Errorf("grant %d: %w", i, err)
			}
			pairs = append(pairs, pair{action: e.Action, scope: scope})
		}
	}

	for _, p := range pairs {
		if err := g.set.Set(ctx, collections.Join3(grantee, p.action, p.scope)); err != nil {
			return err
		}
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(types.GrantEvent(g.module, grantee, entries))
	return nil
}

// RevokeFrom removes the given (action, scope) pairs from grantee. Pairs that
// are not currently granted are silently ignored, so the same revoke can be
// safely re-sent.
//
// Pairs are not checked against the spec: revoking an action the module no
// longer recognises must stay possible, and removing a key that cannot exist
// is a no-op either way.
//
// On success it emits the revoke event itself. ctx must be an SDK context.
func (g Grants) RevokeFrom(ctx context.Context, grantee string, pairs []types.ActionScope) error {
	if len(pairs) > types.MaxGrants {
		return fmt.Errorf("actions length %d exceeds max %d", len(pairs), types.MaxGrants)
	}

	for _, p := range pairs {
		if err := g.set.Remove(ctx, collections.Join3(grantee, p.Action, p.Scope)); err != nil {
			return err
		}
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(types.RevokeEvent(g.module, grantee, pairs))
	return nil
}

// Init imports grants during genesis, enforcing the same spec invariants the
// handlers do. It emits no events.
//
// The module must write the state its scopes refer to before calling this —
// for a scoped module, ScopeExists reads that state.
func (g Grants) Init(ctx context.Context, grants []types.Grant) error {
	for i, gr := range grants {
		if err := g.validatePair(ctx, gr.Action, gr.Scope); err != nil {
			return fmt.Errorf("grant %d: %w", i, err)
		}
		if err := g.set.Set(ctx, collections.Join3(gr.Grantee, gr.Action, gr.Scope)); err != nil {
			return err
		}
	}
	return nil
}

// Export returns every grant in key order, so the export is deterministic.
func (g Grants) Export(ctx context.Context) ([]types.Grant, error) {
	var grants []types.Grant
	err := g.set.Walk(ctx, nil, func(key GrantKey) (bool, error) {
		grants = append(grants, GrantFromKey(key))
		return false, nil
	})
	if err != nil {
		return nil, err
	}
	return grants, nil
}

// validatePair checks an (action, scope) pair against the spec: the action
// must be in the vocabulary, and the scope must satisfy the rules for that
// action — which may differ from the rest of the module, since a spec can
// declare individual actions module-wide.
func (g Grants) validatePair(ctx context.Context, action, scope string) error {
	if !g.spec.HasAction(action) {
		return types.ErrInvalidAction.Wrapf("action %q is not registered for module %q", action, g.module)
	}

	// A module-wide action is required to carry the empty scope, not merely
	// allowed to. That keeps one key form per (grantee, action), so a
	// access check cannot miss a grant stored under a scope the caller did
	// not think to look up.
	if g.spec.IsUnscoped(action) {
		if scope != "" {
			return types.ErrInvalidScope.Wrapf("action %q in module %q is module-wide: scope must be empty, got %q", action, g.module, scope)
		}
		return nil
	}

	if g.spec.ScopeExists == nil {
		return nil
	}
	if scope == "" {
		return types.ErrInvalidScope.Wrapf("module %q scopes action %q: scope must not be empty", g.module, action)
	}
	exists, err := g.spec.ScopeExists(ctx, scope)
	if err != nil {
		return err
	}
	if !exists {
		return types.ErrInvalidScope.Wrapf("scope %q does not exist in module %q", scope, g.module)
	}
	return nil
}
