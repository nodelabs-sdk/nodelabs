package types_test

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func TestValidateGrantEntries(t *testing.T) {
	tooMany := make([]types.ActionScopes, types.MaxGrants+1)
	for i := range tooMany {
		tooMany[i] = types.ActionScopes{Action: "a" + strconv.Itoa(i)}
	}
	tooManyScopes := make([]string, types.MaxGrants+1)
	for i := range tooManyScopes {
		tooManyScopes[i] = "scope" + strconv.Itoa(i)
	}

	tests := []struct {
		name    string
		entries []types.ActionScopes
		wantErr string
	}{
		{
			name:    "scoped entry",
			entries: []types.ActionScopes{{Action: "issue", Scopes: []string{"a", "b"}}},
		},
		{
			name: "module-wide entry has no scopes",
			// Whether an empty scope list is allowed depends on the module's
			// spec, which this stateless check cannot see.
			entries: []types.ActionScopes{{Action: "type.create"}},
		},
		{
			name:    "empty",
			entries: nil,
			wantErr: "must not be empty",
		},
		{
			name:    "invalid action",
			entries: []types.ActionScopes{{Action: "Issue"}},
			wantErr: "grant 0: invalid action",
		},
		{
			name:    "too many entries",
			entries: tooMany,
			wantErr: "exceeds max",
		},
		{
			name:    "too many scopes in one entry",
			entries: []types.ActionScopes{{Action: "issue", Scopes: tooManyScopes}},
			wantErr: "grant 0 scopes length",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := types.ValidateGrantEntries(tc.entries)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestValidateRevokePairs(t *testing.T) {
	tooMany := make([]types.ActionScope, types.MaxGrants+1)
	for i := range tooMany {
		tooMany[i] = types.ActionScope{Action: "a" + strconv.Itoa(i)}
	}

	tests := []struct {
		name    string
		pairs   []types.ActionScope
		wantErr string
	}{
		{
			name:  "scoped pair",
			pairs: []types.ActionScope{{Action: "issue", Scope: "a"}},
		},
		{
			name:  "module-wide pair",
			pairs: []types.ActionScope{{Action: "type.create"}},
		},
		{
			name:    "empty",
			pairs:   nil,
			wantErr: "must not be empty",
		},
		{
			name:    "invalid action",
			pairs:   []types.ActionScope{{Action: "iss ue"}},
			wantErr: "pair 0: invalid action",
		},
		{
			name:    "too many pairs",
			pairs:   tooMany,
			wantErr: "exceeds max",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := types.ValidateRevokePairs(tc.pairs)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
