package types_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

func TestDefaultGenesis(t *testing.T) {
	gs := types.DefaultGenesis()
	require.NoError(t, gs.Validate())
	require.Empty(t, gs.LicenseTypes)
	require.Empty(t, gs.Licenses)
	require.Equal(t, types.FirstLicenseID, gs.NextLicenseId)
}

func TestGenesisValidation(t *testing.T) {
	holder := sample.AccAddress()

	tests := []struct {
		name      string
		genesis   types.GenesisState
		expErr    bool
		expErrMsg string
	}{
		{
			name:    "default genesis",
			genesis: *types.DefaultGenesis(),
			expErr:  false,
		},
		{
			name: "valid with data",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "node", MaxSupply: math.NewInt(100), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "node", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
				NextLicenseId: 2,
			},
			expErr: false,
		},
		{
			name: "duplicate license type",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "dup", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
					{Id: "dup", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
			},
			expErr:    true,
			expErrMsg: "duplicate license type",
		},
		{
			name: "duplicate license id in one type",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, Status: types.StatusActive},
					{Id: 1, Type: "t1", Holder: holder, Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "duplicate license id",
		},
		{
			// Ids are unique chain-wide, so the same id under two different
			// types collides. This was legal when ids were per-type.
			name: "duplicate license id across types",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
					{Id: "t2", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, Status: types.StatusActive},
					{Id: 1, Type: "t2", Holder: holder, Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "duplicate license id",
		},
		{
			name: "license references unknown type",
			genesis: types.GenesisState{
				Licenses: []types.License{
					{Id: 1, Type: "missing", Holder: holder, Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "unknown license type",
		},
		{
			name: "license invalid status",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, Status: types.LicenseStatus(99)},
				},
			},
			expErr:    true,
			expErrMsg: "invalid status",
		},
		{
			name: "license invalid holder",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: "bad", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "invalid holder address",
		},
		{
			name: "license type negative max_supply",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.NewInt(-1), IssuedCount: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
			},
			expErr:    true,
			expErrMsg: "max_supply must not be negative",
		},
		{
			name: "license type nil counter",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), ActiveCount: math.ZeroInt(), RevokedCount: math.ZeroInt()},
				},
			},
			expErr:    true,
			expErrMsg: "issued_count must be set",
		},
		{
			name: "license type negative counter",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.ZeroInt(), ActiveCount: math.NewInt(-1), RevokedCount: math.ZeroInt()},
				},
			},
			expErr:    true,
			expErrMsg: "active_count must not be negative",
		},
		{
			name: "issued_count mismatch",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.NewInt(100), IssuedCount: math.NewInt(2), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "issued_count 2 does not match",
		},
		{
			name: "active_count mismatch",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.NewInt(100), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(0), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "active_count 0 does not match 1",
		},
		{
			name: "revoked_count mismatch",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.NewInt(100), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.NewInt(2)},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "revoked_count 2 does not match",
		},
		{
			name: "license invalid date format",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "01-01-2026", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "YYYY-MM-DD",
		},
		{
			name: "license end_date before start_date",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-06-01", EndDate: "2026-01-01", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "must not be before",
		},
		{
			name: "revoked license without end_date",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.ZeroInt(), RevokedCount: math.NewInt(1)},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusRevoked},
				},
				NextLicenseId: 2,
			},
			expErr:    true,
			expErrMsg: "no end_date",
		},
		{
			// A revoked license's end_date is the revocation date, which may
			// precede start_date, so the date-order check must not apply.
			name: "revoked license with end_date before start_date",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.ZeroInt(), RevokedCount: math.NewInt(1)},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-06-01", EndDate: "2026-01-01", Status: types.StatusRevoked},
				},
				NextLicenseId: 2,
			},
			expErr: false,
		},
		{
			// Equal is not enough: next_license_id is the id to *assign*, so
			// reusing 5 would overwrite the imported license.
			name: "next_license_id not above highest id",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 5, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
				NextLicenseId: 5,
			},
			expErr:    true,
			expErrMsg: "not above the highest license id",
		},
		{
			// The sequence spans every type, so it must clear the highest id
			// anywhere in genesis, not just the highest within one type.
			name: "next_license_id above one type but not another",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
					{Id: "t2", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 3, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
					{Id: 7, Type: "t2", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
				NextLicenseId: 4,
			},
			expErr:    true,
			expErrMsg: "not above the highest license id",
		},
		{
			name: "next_license_id unset",
			genesis: types.GenesisState{
				LicenseTypes: []types.LicenseType{
					{Id: "t1", MaxSupply: math.ZeroInt(), IssuedCount: math.NewInt(1), ActiveCount: math.NewInt(1), RevokedCount: math.ZeroInt()},
				},
				Licenses: []types.License{
					{Id: 1, Type: "t1", Holder: holder, StartDate: "2026-01-01", Status: types.StatusActive},
				},
			},
			expErr:    true,
			expErrMsg: "next_license_id must be at least 1",
		},
		{
			// A chain with no licenses still needs a seeded sequence, so the
			// zero GenesisState is invalid.
			name:      "empty genesis has no sequence",
			genesis:   types.GenesisState{},
			expErr:    true,
			expErrMsg: "next_license_id must be at least 1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.genesis.Validate()
			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
