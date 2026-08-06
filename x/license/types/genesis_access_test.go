package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// genesisWith returns a minimal valid genesis carrying one license type, plus
// the supplied params and grants.
func genesisWith(params types.Params, grants []accesstypes.Grant) types.GenesisState {
	return types.GenesisState{
		LicenseTypes: []types.LicenseType{{
			Id:           "node.license",
			MaxSupply:    math.ZeroInt(),
			IssuedCount:  math.ZeroInt(),
			ActiveCount:  math.ZeroInt(),
			RevokedCount: math.ZeroInt(),
		}},
		NextLicenseId: types.FirstLicenseID,
		Params:        params,
		Grants:        grants,
	}
}

func TestDefaultGenesisAccessFields(t *testing.T) {
	gs := types.DefaultGenesis()
	require.Empty(t, gs.Params.Owner)
	require.Empty(t, gs.Grants)
}

func TestGenesisParamsValidation(t *testing.T) {
	require.NoError(t, genesisWith(types.Params{Owner: sample.AccAddress()}, nil).Validate())
	// An unset owner is legitimate: the chain has not been handed over yet.
	require.NoError(t, genesisWith(types.Params{}, nil).Validate())

	err := genesisWith(types.Params{Owner: "not-an-address"}, nil).Validate()
	require.ErrorContains(t, err, "invalid owner address")
}

// Grants and the license types they scope to now live in one document, so the
// reference can be checked statelessly. Splitting them across two modules made
// this impossible.
func TestGenesisGrantScopeValidation(t *testing.T) {
	addr := sample.AccAddress()

	tests := []struct {
		name    string
		grant   accesstypes.Grant
		wantErr string
	}{
		{
			name:  "scoped grant naming a type in this genesis",
			grant: accesstypes.Grant{Grantee: addr, Action: types.ActionIssue, Scope: "node.license"},
		},
		{
			name:  "module-wide grant carries no scope",
			grant: accesstypes.Grant{Grantee: addr, Action: types.ActionCreateType},
		},
		{
			name:    "scoped grant naming an absent type",
			grant:   accesstypes.Grant{Grantee: addr, Action: types.ActionIssue, Scope: "ghost.license"},
			wantErr: "references unknown license type",
		},
		{
			name:    "scoped grant with no scope",
			grant:   accesstypes.Grant{Grantee: addr, Action: types.ActionRevoke},
			wantErr: "must name a license type",
		},
		{
			name:    "module-wide grant carrying a real scope",
			grant:   accesstypes.Grant{Grantee: addr, Action: types.ActionCreateType, Scope: "node.license"},
			wantErr: "is module-wide",
		},
		{
			name:    "action outside the vocabulary",
			grant:   accesstypes.Grant{Grantee: addr, Action: "transfer", Scope: "node.license"},
			wantErr: "not part of this module's vocabulary",
		},
		{
			name:    "invalid grantee",
			grant:   accesstypes.Grant{Grantee: "nope", Action: types.ActionIssue, Scope: "node.license"},
			wantErr: "invalid grantee address",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := genesisWith(types.Params{}, []accesstypes.Grant{tc.grant}).Validate()
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

func TestGenesisRejectsDuplicateGrants(t *testing.T) {
	addr := sample.AccAddress()
	g := accesstypes.Grant{Grantee: addr, Action: types.ActionIssue, Scope: "node.license"}

	err := genesisWith(types.Params{}, []accesstypes.Grant{g, g}).Validate()
	require.ErrorContains(t, err, "duplicate grant")
}
