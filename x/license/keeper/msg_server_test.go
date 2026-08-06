package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// setupWithOwner returns a license fixture, its msg server, context, and the
// module owner. Grants are usually written through LicenseFixture.Grant, which
// bypasses the owner gate; the owner-gated GrantAccess path has its own tests
// below.
func setupWithOwner(t testing.TB) (*keepertest.LicenseFixture, types.MsgServer, sdk.Context, string) {
	t.Helper()
	f := keepertest.NewLicenseFixture(t)
	return f, keeper.NewMsgServerImpl(f.Keeper), f.Ctx, f.Owner
}

func TestMsgServer(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)
	require.NotNil(t, ms)
	require.NotNil(t, ctx)
	require.NotEmpty(t, f.Keeper)
}

// ---------------------------------------------------------------------------
// CreateLicenseType
// ---------------------------------------------------------------------------

func TestCreateLicenseType(t *testing.T) {
	_, ms, ctx, owner := setupWithOwner(t)

	tests := []struct {
		name      string
		input     *types.MsgCreateLicenseType
		expErr    bool
		expErrMsg string
	}{
		{
			name: "no grant",
			input: &types.MsgCreateLicenseType{
				Creator: sample.AccAddress(),
				Id:      "test.type",
			},
			expErr:    true,
			expErrMsg: "does not hold the type.create grant",
		},
		{
			name: "empty id",
			input: &types.MsgCreateLicenseType{
				Creator: owner,
				Id:      "",
			},
			expErr:    true,
			expErrMsg: "cannot be empty",
		},
		{
			name: "valid",
			input: &types.MsgCreateLicenseType{
				Creator:       owner,
				Id:            "test.type",
				Transferrable: true,
				MaxSupply:     math.NewInt(100),
			},
			expErr: false,
		},
		{
			name: "duplicate",
			input: &types.MsgCreateLicenseType{
				Creator:   owner,
				Id:        "test.type",
				MaxSupply: math.ZeroInt(),
			},
			expErr:    true,
			expErrMsg: "already exists",
		},
		{
			name: "negative max_supply",
			input: &types.MsgCreateLicenseType{
				Creator:   owner,
				Id:        "neg.type",
				MaxSupply: math.NewInt(-1),
			},
			expErr:    true,
			expErrMsg: "max_supply must not be negative",
		},
		{
			name: "nil max_supply",
			input: &types.MsgCreateLicenseType{
				Creator: owner,
				Id:      "nil.type",
			},
			expErr:    true,
			expErrMsg: "max_supply must be set",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.CreateLicenseType(ctx, tc.input)
			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestCreateLicenseTypeByGrant: a grantee holding the module-wide type.create
// grant can create types without owning the module.
func TestCreateLicenseTypeByGrant(t *testing.T) {
	f, ms, ctx, _ := setupWithOwner(t)
	creator := sample.AccAddress()

	// The module-wide grant is stored under the empty scope.
	f.Grant(t, creator, types.ActionCreateType, "")

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:       creator,
		Id:            "delegated.type",
		Transferrable: true,
		MaxSupply:     math.NewInt(10),
	})
	require.NoError(t, err)

	lt, found, err := f.Keeper.GetLicenseType(ctx, "delegated.type")
	require.NoError(t, err)
	require.True(t, found)
	require.True(t, lt.Transferrable)
	require.Equal(t, math.NewInt(10), lt.MaxSupply)

	// HasLicenseType is the accessor x/network consumes. The type is visible to
	// it regardless of who signed the creation: the signer is not recorded, so
	// nothing downstream can be gated on it.
	has, err := f.Keeper.HasLicenseType(ctx, "delegated.type")
	require.NoError(t, err)
	require.True(t, has)

	has, err = f.Keeper.HasLicenseType(ctx, "no.such.type")
	require.NoError(t, err)
	require.False(t, has)
}

// TestCreateLicenseTypeOwnershipAloneIsNotEnough: the module owner creates
// types by holding the grant like anyone else. Strip the grant the fixture
// gives it and creation is refused, even though it still owns the module
// and can grant the right back to itself at will.
func TestCreateLicenseTypeOwnershipAloneIsNotEnough(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)

	f.Ungrant(t, owner, types.ActionCreateType, "")

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   owner,
		Id:        "owned.type",
		MaxSupply: math.ZeroInt(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// Still the owner: re-granting restores the ability.
	f.Grant(t, owner, types.ActionCreateType, "")

	_, err = ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   owner,
		Id:        "owned.type",
		MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
}

// TestCreateLicenseTypeGrantIsModuleWide: the grant only authorizes under the
// empty scope. A grant written against a type id does not carry over, which is
// why the grant store refuses to store one in the first place.
func TestCreateLicenseTypeGrantIsModuleWide(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	creator := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   owner,
		Id:        "existing.type",
		MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)

	// Bypasses the msg path to write the grant GrantAccess would reject.
	f.Grant(t, creator, types.ActionCreateType, "existing.type")

	_, err = ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   creator,
		Id:        "another.type",
		MaxSupply: math.ZeroInt(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// TestCreateLicenseTypeOtherGrantsDoNotAuthorize: holding issue or revoke on
// every existing type does not confer the right to create new ones.
func TestCreateLicenseTypeOtherGrantsDoNotAuthorize(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   owner,
		Id:        "existing.type",
		MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)

	f.Grant(t, issuer, types.ActionIssue, "existing.type")
	f.Grant(t, issuer, types.ActionRevoke, "existing.type")

	_, err = ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   issuer,
		Id:        "another.type",
		MaxSupply: math.ZeroInt(),
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}

// TestGrantCreateTypeThroughMsgServer covers the module's access spec rather
// than the CreateLicenseType handler: the spec must declare type.create
// module-wide, so the real owner-gated grant path accepts it with no scope and
// refuses it with one.
func TestGrantCreateTypeThroughMsgServer(t *testing.T) {
	_, ms, ctx, owner := setupWithOwner(t)
	creator := sample.AccAddress()

	grant := func(action string, scopes ...string) error {
		_, err := ms.GrantAccess(ctx, &types.MsgGrantAccess{
			Owner:   owner,
			Grantee: creator,
			Grants:  []accesstypes.ActionScopes{{Action: action, Scopes: scopes}},
		})
		return err
	}

	require.NoError(t, grant(types.ActionCreateType))

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   creator,
		Id:        "delegated.type",
		MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)

	// Scoping type.create is refused even to a type that now exists.
	require.ErrorIs(t, grant(types.ActionCreateType, "delegated.type"), accesstypes.ErrInvalidScope)

	// The exemption does not leak to the scoped actions.
	require.ErrorIs(t, grant(types.ActionIssue), accesstypes.ErrInvalidScope)
	require.NoError(t, grant(types.ActionIssue, "delegated.type"))
}

// TestUpdateLicenseTypeStillOwnerOnly: type.create authorizes creation only.
// Updating a type remains the module owner's alone.
func TestUpdateLicenseTypeStillOwnerOnly(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	creator := sample.AccAddress()

	f.Grant(t, creator, types.ActionCreateType, "")

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator:   creator,
		Id:        "delegated.type",
		MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)

	// Not even over the type it just created.
	_, err = ms.UpdateLicenseType(ctx, &types.MsgUpdateLicenseType{
		Owner:         creator,
		Id:            "delegated.type",
		Transferrable: true,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	_, err = ms.UpdateLicenseType(ctx, &types.MsgUpdateLicenseType{
		Owner:         owner,
		Id:            "delegated.type",
		Transferrable: true,
	})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// IssueLicenses
// ---------------------------------------------------------------------------

func TestIssueLicenses(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "node", MaxSupply: math.NewInt(10),
	})
	require.NoError(t, err)

	f.Grant(t, issuer, types.ActionIssue, "node")

	tests := []struct {
		name      string
		input     *types.MsgIssueLicenses
		expErr    bool
		expErrMsg string
		expCount  int
	}{
		{
			name: "empty entries",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{},
			},
			expErr:    true,
			expErrMsg: "must not be empty",
		},
		{
			name: "no grant",
			input: &types.MsgIssueLicenses{
				Issuer: sample.AccAddress(), Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", Count: 1},
				},
			},
			expErr:    true,
			expErrMsg: "does not hold the issue grant",
		},
		{
			name: "invalid holder",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: "bad", StartDate: "2026-01-01", Count: 1},
				},
			},
			expErr:    true,
			expErrMsg: "invalid holder address",
		},
		{
			name: "missing start_date",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "", Count: 1},
				},
			},
			expErr:    true,
			expErrMsg: "start_date is required",
		},
		{
			name: "bad date format",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "01-01-2026", Count: 1},
				},
			},
			expErr:    true,
			expErrMsg: "YYYY-MM-DD",
		},
		{
			name: "end_date before start_date",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "2026-06-01", EndDate: "2026-01-01", Count: 1},
				},
			},
			expErr:    true,
			expErrMsg: "must not be before",
		},
		{
			name: "count zero",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01"},
				},
			},
			expErr:    true,
			expErrMsg: "count must be greater than zero",
		},
		{
			name: "valid single",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", EndDate: "2027-01-01", Count: 1},
				},
			},
			expErr:   false,
			expCount: 1,
		},
		{
			name: "valid with count=3",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", Count: 3},
				},
			},
			expErr:   false,
			expCount: 3,
		},
		{
			name: "max supply exceeded",
			input: &types.MsgIssueLicenses{
				Issuer: issuer, Entries: []types.IssueLicenseEntry{
					{LicenseTypeId: "node", Holder: holder, StartDate: "2026-01-01", Count: 10},
				},
			},
			expErr:    true,
			expErrMsg: "exceed max supply",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := ms.IssueLicenses(ctx, tc.input)
			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
				require.Len(t, resp.Ids, tc.expCount)
			}
		})
	}

	lt, found, err := f.Keeper.GetLicenseType(ctx, "node")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, math.NewInt(4), lt.IssuedCount)
}

// TestMaxSupplyBoundsOutstandingNotLifetime pins the supply semantic that a
// chargeback depends on: max_supply caps licenses currently outstanding, so
// revoking one returns its slot to the pool. A license sold and then charged
// back must stop consuming supply — under a lifetime cap it would burn a slot
// permanently, and a type would run out of supply while holding none.
//
// The trade-off this encodes: lifetime issued_count is allowed to exceed
// max_supply, so the cap is not a promise about how many were ever sold.
func TestMaxSupplyBoundsOutstandingNotLifetime(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()
	revoker := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "capped", MaxSupply: math.NewInt(2),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "capped")
	f.Grant(t, revoker, types.ActionRevoke, "capped")

	issue := func(count uint64) ([]uint64, error) {
		resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
			Issuer: issuer, Entries: []types.IssueLicenseEntry{
				{LicenseTypeId: "capped", Holder: holder, StartDate: "2026-01-01", Count: count},
			},
		})
		if err != nil {
			return nil, err
		}
		return resp.Ids, nil
	}

	// Fill the cap, then confirm it binds.
	issuedIDs, err := issue(2)
	require.NoError(t, err)
	_, err = issue(1)
	require.ErrorIs(t, err, types.ErrMaxSupplyReached)

	lt, _, err := f.Keeper.GetLicenseType(ctx, "capped")
	require.NoError(t, err)
	require.Equal(t, math.NewInt(2), lt.ActiveCount)
	require.Equal(t, math.NewInt(2), lt.IssuedCount)

	// Chargeback: revoking one frees exactly one slot.
	revResp, err := ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: revoker, LicenseTypeId: "capped", LicenseIds: []uint64{issuedIDs[1]},
	})
	require.NoError(t, err)

	lt, _, err = f.Keeper.GetLicenseType(ctx, "capped")
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1), lt.ActiveCount)
	require.Equal(t, math.NewInt(1), lt.RevokedCount)
	require.Equal(t, math.NewInt(2), lt.IssuedCount, "issued_count must keep counting lifetime issuance")

	// One slot free, not two: reissuing 2 still exceeds, reissuing 1 succeeds.
	_, err = issue(2)
	require.ErrorIs(t, err, types.ErrMaxSupplyReached)
	reissuedIDs, err := issue(1)
	require.NoError(t, err)

	lt, _, err = f.Keeper.GetLicenseType(ctx, "capped")
	require.NoError(t, err)
	require.Equal(t, math.NewInt(2), lt.ActiveCount, "outstanding is back at the cap")
	require.Equal(t, math.NewInt(3), lt.IssuedCount, "lifetime issuance exceeds max_supply by design")

	// And the cap binds again now that the freed slot is used.
	_, err = issue(1)
	require.ErrorIs(t, err, types.ErrMaxSupplyReached)

	// The reissued license got a fresh id rather than reusing the revoked
	// one's — ids come from a monotonic sequence, so freeing supply never
	// recycles an id.
	require.Len(t, reissuedIDs, 1)
	require.NotContains(t, issuedIDs, reissuedIDs[0], "reissue must not reuse an original id")
	require.NotContains(t, revResp.Ids, reissuedIDs[0], "reissue must not reuse the revoked id")
	require.Greater(t, reissuedIDs[0], issuedIDs[len(issuedIDs)-1], "ids only ever climb")
}

// TestIssueLicensesMultipleEntries covers the multi-entry behavior: entries
// can target different holders and license types, per-entry counts accumulate
// against the supply cap, and the signer needs the "issue" grant for every
// referenced type.
func TestIssueLicensesMultipleEntries(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()
	holder1 := sample.AccAddress()
	holder2 := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "capped", MaxSupply: math.NewInt(5),
	})
	require.NoError(t, err)
	_, err = ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "open", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	_, err = ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "ungranted", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)

	f.Grant(t, issuer, types.ActionIssue, "capped")
	f.Grant(t, issuer, types.ActionIssue, "open")

	// Signer must hold the issue grant for every type referenced by the entries.
	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "open", Holder: holder1, StartDate: "2026-01-01", Count: 1},
			{LicenseTypeId: "ungranted", Holder: holder1, StartDate: "2026-01-01", Count: 1},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not hold the issue grant for license type ungranted")
	// Nothing was issued for the granted entry either.
	lt, _, err := f.Keeper.GetLicenseType(ctx, "open")
	require.NoError(t, err)
	require.True(t, lt.IssuedCount.IsZero())

	// Counts for entries referencing the same type accumulate against the cap.
	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "capped", Holder: holder1, StartDate: "2026-01-01", Count: 3},
			{LicenseTypeId: "capped", Holder: holder2, StartDate: "2026-01-01", Count: 3},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceed max supply")
	lt, _, err = f.Keeper.GetLicenseType(ctx, "capped")
	require.NoError(t, err)
	require.True(t, lt.IssuedCount.IsZero(), "failed batch must not issue anything")

	// Valid mixed batch across types and holders.
	resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "capped", Holder: holder1, StartDate: "2026-01-01", EndDate: "2027-01-01", Count: 2},
			{LicenseTypeId: "capped", Holder: holder2, StartDate: "2026-02-01", Count: 3},
			{LicenseTypeId: "open", Holder: holder2, StartDate: "2026-03-01", Count: 1},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Ids, 6, "ids are flattened in entry order")

	// Per-type counters reflect the aggregate issuance.
	lt, _, err = f.Keeper.GetLicenseType(ctx, "capped")
	require.NoError(t, err)
	require.Equal(t, math.NewInt(5), lt.IssuedCount)
	lt, _, err = f.Keeper.GetLicenseType(ctx, "open")
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1), lt.IssuedCount)

	// Each holder got the licenses from their entries.
	l, found, err := f.Keeper.GetLicense(ctx, resp.Ids[0])
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, holder1, l.Holder)
	l, found, err = f.Keeper.GetLicense(ctx, resp.Ids[2])
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, holder2, l.Holder)
	l, found, err = f.Keeper.GetLicense(ctx, resp.Ids[5])
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, holder2, l.Holder)
}

// ---------------------------------------------------------------------------
// RevokeLicenses
// ---------------------------------------------------------------------------

func TestRevokeLicenses(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()
	revoker := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "rev", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "rev")
	f.Grant(t, revoker, types.ActionRevoke, "rev")

	// Issue 3 licenses to the same holder.
	resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "rev", Holder: holder, StartDate: "2026-01-01", Count: 3},
		},
	})
	require.NoError(t, err)
	require.Len(t, resp.Ids, 3)

	tests := []struct {
		name      string
		input     *types.MsgRevokeLicenses
		expErr    bool
		expErrMsg string
	}{
		{
			name:      "no grant",
			input:     &types.MsgRevokeLicenses{Revoker: sample.AccAddress(), LicenseTypeId: "rev", LicenseIds: []uint64{resp.Ids[0]}},
			expErr:    true,
			expErrMsg: "does not hold the revoke grant",
		},
		{
			name:      "unknown license id",
			input:     &types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "rev", LicenseIds: []uint64{9999}},
			expErr:    true,
			expErrMsg: "license 9999 not found",
		},
		{
			name:      "duplicate license ids",
			input:     &types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "rev", LicenseIds: []uint64{resp.Ids[0], resp.Ids[0]}},
			expErr:    true,
			expErrMsg: "duplicate license id",
		},
		{
			// One bad id fails the whole batch; the success case below
			// confirms resp.Ids[0] is still active afterwards.
			name:      "atomic — one bad id revokes nothing",
			input:     &types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "rev", LicenseIds: []uint64{resp.Ids[0], 9999}},
			expErr:    true,
			expErrMsg: "license 9999 not found",
		},
		{
			name:   "revoke 2 by id",
			input:  &types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "rev", LicenseIds: []uint64{resp.Ids[2], resp.Ids[1]}},
			expErr: false,
		},
		{
			name:      "already revoked",
			input:     &types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "rev", LicenseIds: []uint64{resp.Ids[2]}},
			expErr:    true,
			expErrMsg: "is not active",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			revokeResp, err := ms.RevokeLicenses(ctx, tc.input)
			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
				require.Len(t, revokeResp.Ids, 2)
				// The response echoes the requested ids in request order.
				require.Equal(t, resp.Ids[2], revokeResp.Ids[0])
				require.Equal(t, resp.Ids[1], revokeResp.Ids[1])

				// Verify revoked licenses record the revocation date in
				// end_date (empty at issuance here — none was set).
				for _, id := range revokeResp.Ids {
					license, found, _ := f.Keeper.GetLicense(ctx, id)
					require.True(t, found)
					require.Equal(t, types.StatusRevoked, license.Status)
					require.Equal(t, ctx.BlockTime().Format("2006-01-02"), license.EndDate, "end_date records the revocation date")
				}

				// Verify the remaining license is still active.
				license, found, _ := f.Keeper.GetLicense(ctx, resp.Ids[0])
				require.True(t, found)
				require.Equal(t, types.StatusActive, license.Status)

				// Verify counters.
				lt, _, _ := f.Keeper.GetLicenseType(ctx, "rev")
				require.Equal(t, math.NewInt(3), lt.IssuedCount)
				require.Equal(t, math.NewInt(1), lt.ActiveCount)
				require.Equal(t, math.NewInt(2), lt.RevokedCount)
			}
		})
	}
}

// TestRevokeLicensesOverwritesEndDate: revocation replaces the issued
// end_date with the revocation date (block date).
func TestRevokeLicensesOverwritesEndDate(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	ctx = ctx.WithBlockTime(time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC))
	admin := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "ed", MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, admin, types.ActionIssue, "ed")
	f.Grant(t, admin, types.ActionRevoke, "ed")

	resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: admin, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "ed", Holder: holder, StartDate: "2026-01-01", EndDate: "2027-01-01", Count: 1},
		},
	})
	require.NoError(t, err)

	_, err = ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: admin, LicenseTypeId: "ed", LicenseIds: resp.Ids,
	})
	require.NoError(t, err)

	l, found, err := f.Keeper.GetLicense(ctx, resp.Ids[0])
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, types.StatusRevoked, l.Status)
	require.Equal(t, "2026-06-15", l.EndDate, "issued end_date is overwritten with the revocation date")
}

// ---------------------------------------------------------------------------
// Global license ids
// ---------------------------------------------------------------------------

// setupTwoTypes creates two transferrable license types and grants the owner
// issue rights on both.
func setupTwoTypes(t testing.TB) (*keepertest.LicenseFixture, types.MsgServer, sdk.Context, string) {
	t.Helper()
	f, ms, ctx, owner := setupWithOwner(t)

	for _, id := range []string{"type.a", "type.b"} {
		_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
			Creator: owner, Id: id, Transferrable: true, MaxSupply: math.ZeroInt(),
		})
		require.NoError(t, err)
		f.Grant(t, owner, types.ActionIssue, id)
	}
	return f, ms, ctx, owner
}

// TestLicenseIDsAreGlobalAcrossTxs: the sequence spans license types and
// transactions. Under per-type counters this would be {1,2}, {1,2}, {3}.
func TestLicenseIDsAreGlobalAcrossTxs(t *testing.T) {
	f, ms, ctx, owner := setupTwoTypes(t)
	holder := sample.AccAddress()

	issue := func(typeID string, count uint64) []uint64 {
		t.Helper()
		resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
			Issuer: owner, Entries: []types.IssueLicenseEntry{
				{LicenseTypeId: typeID, Holder: holder, StartDate: "2026-01-01", Count: count},
			},
		})
		require.NoError(t, err)
		return resp.Ids
	}

	require.Equal(t, []uint64{1, 2}, issue("type.a", 2))
	require.Equal(t, []uint64{3, 4}, issue("type.b", 2))
	require.Equal(t, []uint64{5}, issue("type.a", 1))

	// The id alone resolves the type — no type argument anywhere here.
	for id, wantType := range map[uint64]string{1: "type.a", 3: "type.b", 5: "type.a"} {
		l, found, err := f.Keeper.GetLicense(ctx, id)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, wantType, l.Type, "license %d", id)
	}
}

// TestLicenseIDsAreGlobalWithinOneBatch: entries in a single message draw from
// the same sequence, so ids stay unique when a batch spans types.
func TestLicenseIDsAreGlobalWithinOneBatch(t *testing.T) {
	f, ms, ctx, owner := setupTwoTypes(t)
	holder := sample.AccAddress()

	resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: owner, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "type.a", Holder: holder, StartDate: "2026-01-01", Count: 1},
			{LicenseTypeId: "type.b", Holder: holder, StartDate: "2026-01-01", Count: 2},
			{LicenseTypeId: "type.a", Holder: holder, StartDate: "2026-01-01", Count: 1},
		},
	})
	require.NoError(t, err)
	require.Equal(t, []uint64{1, 2, 3, 4}, resp.Ids, "flattened in entry order, one sequence")

	wantTypes := []string{"type.a", "type.b", "type.b", "type.a"}
	for i, id := range resp.Ids {
		l, found, err := f.Keeper.GetLicense(ctx, id)
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, wantTypes[i], l.Type, "license %d", id)
	}
}

// TestLicenseIDsDoNotAliasAcrossTypes: an id resolves to exactly one license,
// whatever its type. Under per-type ids both licenses here would have been id
// 1, so a lookup that lost the type argument would have hit the wrong one.
func TestLicenseIDsDoNotAliasAcrossTypes(t *testing.T) {
	f, ms, ctx, owner := setupTwoTypes(t)
	holder := sample.AccAddress()

	resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: owner, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "type.a", Holder: holder, StartDate: "2026-01-01", Count: 1},
			{LicenseTypeId: "type.b", Holder: holder, StartDate: "2026-01-01", Count: 1},
		},
	})
	require.NoError(t, err)
	aID, bID := resp.Ids[0], resp.Ids[1]
	require.NotEqual(t, aID, bID)

	// Each id resolves to its own record, and the type comes off the value
	// rather than being supplied by the caller.
	a, found, err := f.Keeper.GetLicense(ctx, aID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "type.a", a.Type)

	b, found, err := f.Keeper.GetLicense(ctx, bID)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "type.b", b.Type)

	// A revoke that names the other type's id must fail the type check.
	f.Grant(t, owner, types.ActionRevoke, "type.a")
	_, err = ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: owner, LicenseTypeId: "type.a", LicenseIds: []uint64{bID},
	})
	require.ErrorContains(t, err, "is of type")

	// Revoking one type's license must not touch the other's, even though a
	// per-type id scheme would have given both licenses the same id.
	_, err = ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: owner, LicenseTypeId: "type.a", LicenseIds: []uint64{aID},
	})
	require.NoError(t, err)

	a, _, err = f.Keeper.GetLicense(ctx, aID)
	require.NoError(t, err)
	require.Equal(t, types.StatusRevoked, a.Status)

	b, _, err = f.Keeper.GetLicense(ctx, bID)
	require.NoError(t, err)
	require.Equal(t, types.StatusActive, b.Status, "the other type's license must be untouched")
}

// TestRevokedLicenseStaysListedByType: LicensesByType covers active and
// revoked alike, so the by-type index must survive revocation.
func TestRevokedLicenseStaysListedByType(t *testing.T) {
	f, ms, ctx, owner := setupTwoTypes(t)
	holder := sample.AccAddress()
	f.Grant(t, owner, types.ActionRevoke, "type.a")
	q := keeper.NewQuerier(f.Keeper)

	resp, err := ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: owner, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "type.a", Holder: holder, StartDate: "2026-01-01", Count: 2},
			{LicenseTypeId: "type.b", Holder: holder, StartDate: "2026-01-01", Count: 1},
		},
	})
	require.NoError(t, err)

	_, err = ms.RevokeLicenses(ctx, &types.MsgRevokeLicenses{
		Revoker: owner, LicenseTypeId: "type.a", LicenseIds: []uint64{resp.Ids[0]},
	})
	require.NoError(t, err)

	byType, err := q.LicensesByType(ctx, &types.QueryLicensesByTypeRequest{TypeId: "type.a"})
	require.NoError(t, err)
	require.Len(t, byType.Licenses, 2, "revoked licenses stay listed by type")
	for _, l := range byType.Licenses {
		require.Equal(t, "type.a", l.Type, "the index must not leak other types")
	}
}

// TestIssueLicensesSupplyCheckIsUnsigned guards the supply-cap arithmetic:
// a count with the high bit set must not wrap negative and silently bypass
// the MaxSupply check.
func TestIssueLicensesSupplyCheckIsUnsigned(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "lim", Transferrable: false, MaxSupply: math.NewInt(100),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "lim")

	// 1<<63 is the smallest uint64 value that wraps to a negative int64.
	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "lim", Holder: holder, StartDate: "2026-01-01", Count: 1 << 63},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceed max supply")
}

// TestIssueLicensesEntriesCap ensures IssueLicenses rejects entry lists
// larger than MaxIssueBatchSize.
func TestIssueLicensesEntriesCap(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()
	holder := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "cap", Transferrable: false, MaxSupply: math.ZeroInt(),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "cap")

	entries := make([]types.IssueLicenseEntry, types.MaxIssueBatchSize+1)
	for i := range entries {
		entries[i] = types.IssueLicenseEntry{LicenseTypeId: "cap", Holder: holder, StartDate: "2026-01-01", Count: 1}
	}

	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: entries,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "exceeds max batch size")
}

// ---------------------------------------------------------------------------
// UpdateLicenseType
// ---------------------------------------------------------------------------

func TestUpdateLicenseType(t *testing.T) {
	f, ms, ctx, owner := setupWithOwner(t)
	issuer := sample.AccAddress()

	_, err := ms.CreateLicenseType(ctx, &types.MsgCreateLicenseType{
		Creator: owner, Id: "lt1", Transferrable: false, MaxSupply: math.NewInt(100),
	})
	require.NoError(t, err)
	f.Grant(t, issuer, types.ActionIssue, "lt1")
	_, err = ms.IssueLicenses(ctx, &types.MsgIssueLicenses{
		Issuer: issuer, Entries: []types.IssueLicenseEntry{
			{LicenseTypeId: "lt1", Holder: sample.AccAddress(), StartDate: "2026-01-01", Count: 5},
		},
	})
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     *types.MsgUpdateLicenseType
		expErr    bool
		expErrMsg string
	}{
		{
			name:      "non-owner",
			input:     &types.MsgUpdateLicenseType{Owner: sample.AccAddress(), Id: "lt1", Transferrable: true},
			expErr:    true,
			expErrMsg: "is not the owner of module",
		},
		{
			name:      "not found",
			input:     &types.MsgUpdateLicenseType{Owner: owner, Id: "missing", Transferrable: true},
			expErr:    true,
			expErrMsg: "not found",
		},
		{
			name:   "valid update",
			input:  &types.MsgUpdateLicenseType{Owner: owner, Id: "lt1", Transferrable: true},
			expErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.UpdateLicenseType(ctx, tc.input)
			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}

	// The update only toggles transferrability; max_supply keeps its
	// creation-time value.
	lt, err := f.Keeper.LicenseTypes.Get(ctx, "lt1")
	require.NoError(t, err)
	require.True(t, lt.Transferrable)
	require.Equal(t, "100", lt.MaxSupply.String())
}
