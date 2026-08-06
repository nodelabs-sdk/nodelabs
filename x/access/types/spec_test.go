package types_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func alwaysExists(_ context.Context, _ string) (bool, error) { return true, nil }

func TestValidateAction(t *testing.T) {
	valid := []string{"issue", "type.create", "wallet_create", "node-type", "a1"}
	for _, s := range valid {
		require.NoError(t, types.ValidateAction(s), s)
	}

	invalid := []string{"", "Issue", "type create", "issue,revoke", "issue:scope", "iss/ue", "issué"}
	for _, s := range invalid {
		require.Error(t, types.ValidateAction(s), s)
	}
}

func TestSpecValidate(t *testing.T) {
	tests := []struct {
		name    string
		spec    types.Spec
		wantErr string
	}{
		{
			name: "minimal",
			spec: types.Spec{Actions: []string{"issue"}},
		},
		{
			name: "scoped with unscoped action",
			spec: types.Spec{
				Actions:     []string{"issue", "revoke", "type.create"},
				ScopeExists: alwaysExists,
				Unscoped:    []string{"type.create"},
			},
		},
		{
			name:    "no actions",
			spec:    types.Spec{},
			wantErr: "at least one action",
		},
		{
			name:    "invalid action name",
			spec:    types.Spec{Actions: []string{"Issue"}},
			wantErr: "invalid action",
		},
		{
			name:    "duplicate action",
			spec:    types.Spec{Actions: []string{"issue", "issue"}},
			wantErr: "duplicate action",
		},
		{
			name: "unscoped without scope validator",
			spec: types.Spec{
				Actions:  []string{"issue", "type.create"},
				Unscoped: []string{"type.create"},
			},
			wantErr: "without a scope validator",
		},
		{
			name: "unscoped action outside vocabulary",
			spec: types.Spec{
				Actions:     []string{"issue"},
				ScopeExists: alwaysExists,
				Unscoped:    []string{"type.create"},
			},
			wantErr: "not in the vocabulary",
		},
		{
			name: "duplicate unscoped action",
			spec: types.Spec{
				Actions:     []string{"issue", "type.create"},
				ScopeExists: alwaysExists,
				Unscoped:    []string{"type.create", "type.create"},
			},
			wantErr: "duplicate unscoped action",
		},
		{
			name: "every action unscoped",
			spec: types.Spec{
				Actions:     []string{"issue", "type.create"},
				ScopeExists: alwaysExists,
				Unscoped:    []string{"issue", "type.create"},
			},
			wantErr: "drop ScopeExists instead",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.spec.Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestSpecHasAction(t *testing.T) {
	spec := types.Spec{Actions: []string{"issue", "revoke"}}

	require.True(t, spec.HasAction("issue"))
	require.True(t, spec.HasAction("revoke"))
	require.False(t, spec.HasAction("type.create"))
	require.False(t, spec.HasAction(""))
}

func TestSpecIsUnscoped(t *testing.T) {
	scoped := types.Spec{
		Actions:     []string{"issue", "type.create"},
		ScopeExists: alwaysExists,
		Unscoped:    []string{"type.create"},
	}
	require.True(t, scoped.IsUnscoped("type.create"))
	require.False(t, scoped.IsUnscoped("issue"))

	// A module with no scope validator is module-wide throughout, so nothing
	// is singled out as unscoped.
	open := types.Spec{Actions: []string{"operate"}}
	require.False(t, open.IsUnscoped("operate"))
}

func TestSpecSortedAccessorsDoNotMutate(t *testing.T) {
	spec := types.Spec{
		Actions:     []string{"revoke", "issue", "type.create"},
		ScopeExists: alwaysExists,
		Unscoped:    []string{"type.create"},
	}

	require.Equal(t, []string{"issue", "revoke", "type.create"}, spec.SortedActions())
	require.Equal(t, []string{"type.create"}, spec.SortedUnscoped())

	// The spec's own slices keep their declaration order.
	require.Equal(t, []string{"revoke", "issue", "type.create"}, spec.Actions)
}
