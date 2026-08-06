package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func TestGrantEventEncoding(t *testing.T) {
	grantee := sample.AccAddress()

	ev := types.GrantEvent("license", grantee, []types.ActionScopes{
		{Action: "issue", Scopes: []string{"a", "b"}},
		{Action: "revoke", Scopes: []string{"a"}},
	})

	require.Equal(t, types.EventTypeGrantAccess, ev.Type)

	got := map[string]string{}
	for _, a := range ev.Attributes {
		got[a.Key] = a.Value
	}

	require.Equal(t, "license", got[types.AttributeKeyModule])
	require.Equal(t, grantee, got[types.AttributeKeyGrantee])
	require.Equal(t, "issue,revoke", got[types.AttributeKeyActions])
	// Scopes are comma-joined within an entry and semicolon-joined between
	// entries, so the two lists stay positionally paired even when one entry
	// carries several scopes.
	require.Equal(t, "a,b;a", got[types.AttributeKeyScopes])
}

func TestGrantEventModuleWide(t *testing.T) {
	grantee := sample.AccAddress()

	ev := types.GrantEvent("network", grantee, []types.ActionScopes{
		{Action: "wallet.create"},
	})

	got := map[string]string{}
	for _, a := range ev.Attributes {
		got[a.Key] = a.Value
	}

	require.Equal(t, "wallet.create", got[types.AttributeKeyActions])
	require.Empty(t, got[types.AttributeKeyScopes])
}

func TestRevokeEventEncoding(t *testing.T) {
	grantee := sample.AccAddress()

	ev := types.RevokeEvent("license", grantee, []types.ActionScope{
		{Action: "issue", Scope: "a"},
		{Action: "type.create"},
	})

	require.Equal(t, types.EventTypeRevokeAccess, ev.Type)

	got := map[string]string{}
	for _, a := range ev.Attributes {
		got[a.Key] = a.Value
	}

	require.Equal(t, "issue,type.create", got[types.AttributeKeyActions])
	// One scope per action, positionally paired; the module-wide pair
	// contributes an empty entry rather than being dropped.
	require.Equal(t, "a,", got[types.AttributeKeyScopes])
}

func TestTransferOwnershipEvent(t *testing.T) {
	owner := sample.AccAddress()

	ev := types.TransferOwnershipEvent("license", owner)
	require.Equal(t, types.EventTypeTransferOwnership, ev.Type)

	got := map[string]string{}
	for _, a := range ev.Attributes {
		got[a.Key] = a.Value
	}

	require.Equal(t, "license", got[types.AttributeKeyModule])
	require.Equal(t, owner, got[types.AttributeKeyOwner])
}
