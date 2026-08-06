package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func TestValidateGrants(t *testing.T) {
	addr := sample.AccAddress()
	other := sample.AccAddress()

	tests := []struct {
		name    string
		grants  []types.Grant
		wantErr string
	}{
		{
			name:   "empty",
			grants: nil,
		},
		{
			name: "distinct grants",
			grants: []types.Grant{
				{Grantee: addr, Action: "issue", Scope: "a"},
				{Grantee: addr, Action: "issue", Scope: "b"},
				{Grantee: addr, Action: "revoke", Scope: "a"},
				{Grantee: other, Action: "issue", Scope: "a"},
				{Grantee: addr, Action: "type.create"},
			},
		},
		{
			name:    "invalid grantee",
			grants:  []types.Grant{{Grantee: "not-an-address", Action: "issue"}},
			wantErr: "invalid grantee address",
		},
		{
			name:    "invalid action",
			grants:  []types.Grant{{Grantee: addr, Action: "ISSUE"}},
			wantErr: "grant 0: invalid action",
		},
		{
			name: "duplicate key",
			grants: []types.Grant{
				{Grantee: addr, Action: "issue", Scope: "a"},
				{Grantee: addr, Action: "issue", Scope: "a"},
			},
			wantErr: "duplicate grant",
		},
		{
			name: "scope alone distinguishes otherwise equal grants",
			grants: []types.Grant{
				{Grantee: addr, Action: "issue"},
				{Grantee: addr, Action: "issue", Scope: "a"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := types.ValidateGrants(tc.grants)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
