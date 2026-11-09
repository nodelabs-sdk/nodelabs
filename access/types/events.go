package types

import (
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	EventTypeGrantAccess       = "grant_access"
	EventTypeRevokeAccess      = "revoke_access"
	EventTypeTransferOwnership = "transfer_ownership"

	// AttributeKeyModule names the module whose grant store was touched. Both
	// modules emit the same event types, so indexers need it to tell them
	// apart.
	AttributeKeyModule  = "module"
	AttributeKeyOwner   = "owner"
	AttributeKeyGrantee = "grantee"
	AttributeKeyActions = "actions"
	AttributeKeyScopes  = "scopes"
)

// GrantEvent builds the event for a successful grant. Actions are comma-joined
// in entry order; scopes are comma-joined within an entry and semicolon-joined
// between entries, so the two attributes stay positionally paired even when an
// entry carries several scopes.
func GrantEvent(module, grantee string, entries []ActionScopes) sdk.Event {
	actions := make([]string, 0, len(entries))
	scopeLists := make([]string, 0, len(entries))
	for _, e := range entries {
		actions = append(actions, e.Action)
		scopeLists = append(scopeLists, strings.Join(e.Scopes, ","))
	}

	return sdk.NewEvent(
		EventTypeGrantAccess,
		sdk.NewAttribute(AttributeKeyModule, module),
		sdk.NewAttribute(AttributeKeyGrantee, grantee),
		sdk.NewAttribute(AttributeKeyActions, strings.Join(actions, ",")),
		sdk.NewAttribute(AttributeKeyScopes, strings.Join(scopeLists, ";")),
	)
}

// RevokeEvent builds the event for a successful revoke. Each pair contributes
// one entry to each list, so actions and scopes are positionally paired.
func RevokeEvent(module, grantee string, pairs []ActionScope) sdk.Event {
	actions := make([]string, 0, len(pairs))
	scopes := make([]string, 0, len(pairs))
	for _, p := range pairs {
		actions = append(actions, p.Action)
		scopes = append(scopes, p.Scope)
	}

	return sdk.NewEvent(
		EventTypeRevokeAccess,
		sdk.NewAttribute(AttributeKeyModule, module),
		sdk.NewAttribute(AttributeKeyGrantee, grantee),
		sdk.NewAttribute(AttributeKeyActions, strings.Join(actions, ",")),
		sdk.NewAttribute(AttributeKeyScopes, strings.Join(scopes, ",")),
	)
}

// TransferOwnershipEvent builds the event for a module owner handoff. Owner is
// the new owner.
func TransferOwnershipEvent(module, owner string) sdk.Event {
	return sdk.NewEvent(
		EventTypeTransferOwnership,
		sdk.NewAttribute(AttributeKeyModule, module),
		sdk.NewAttribute(AttributeKeyOwner, owner),
	)
}
