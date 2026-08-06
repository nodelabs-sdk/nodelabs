package licenseprecompile

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// seedLicenseType is a test helper that writes a license type directly via the
// keeper so query tests don't have to round-trip through tx methods first.
func seedLicenseType(t *testing.T, f *testFixture, id string, transferrable bool, maxSupply int64) {
	t.Helper()
	lt := licensetypes.LicenseType{
		Id:            id,
		Transferrable: transferrable,
		MaxSupply:     math.NewInt(maxSupply),
		IssuedCount:   math.ZeroInt(),
		ActiveCount:   math.ZeroInt(),
		RevokedCount:  math.ZeroInt(),
	}
	require.NoError(t, f.keeper.LicenseTypes.Set(f.ctx, id, lt))
}

func TestQueryLicenseType(t *testing.T) {
	f := newTestFixture(t)
	seedLicenseType(t, f, "type.a", true, 100)

	method := ABI.Methods[LicenseTypeMethod]
	bz, err := f.precompile.LicenseType(f.ctx, &method, []interface{}{"type.a"})
	require.NoError(t, err)

	out, err := method.Outputs.Unpack(bz)
	require.NoError(t, err)

	got := out[0].(struct {
		Id            string   `json:"id"`
		Transferrable bool     `json:"transferrable"`
		MaxSupply     *big.Int `json:"maxSupply"`
		IssuedCount   *big.Int `json:"issuedCount"`
		ActiveCount   *big.Int `json:"activeCount"`
		RevokedCount  *big.Int `json:"revokedCount"`
	})
	require.Equal(t, "type.a", got.Id)
	require.True(t, got.Transferrable)
	require.Zero(t, got.MaxSupply.Cmp(big.NewInt(100)))
	require.Zero(t, got.IssuedCount.Sign())
}

func TestQueryLicenseTypeNotFound(t *testing.T) {
	f := newTestFixture(t)
	method := ABI.Methods[LicenseTypeMethod]

	_, err := f.precompile.LicenseType(f.ctx, &method, []interface{}{"nope"})
	require.Error(t, err)
}

func TestQueryLicenseTypes(t *testing.T) {
	f := newTestFixture(t)
	seedLicenseType(t, f, "type.a", true, 10)
	seedLicenseType(t, f, "type.b", false, 0)

	method := ABI.Methods[LicenseTypesMethod]
	bz, err := f.precompile.LicenseTypes(f.ctx, &method, nil)
	require.NoError(t, err)

	out, err := method.Outputs.Unpack(bz)
	require.NoError(t, err)
	got := out[0].([]struct {
		Id            string   `json:"id"`
		Transferrable bool     `json:"transferrable"`
		MaxSupply     *big.Int `json:"maxSupply"`
		IssuedCount   *big.Int `json:"issuedCount"`
		ActiveCount   *big.Int `json:"activeCount"`
		RevokedCount  *big.Int `json:"revokedCount"`
	})
	require.Len(t, got, 2)
	ids := []string{got[0].Id, got[1].Id}
	require.ElementsMatch(t, []string{"type.a", "type.b"}, ids)
}

// TestQueryLicenseAndByHolder exercises the License, LicensesByType, LicensesByHolder
// and LicensesByHolderAndType queries with a license issued via the tx path so we
// also cover the holder bech32→hex round trip in outputs.
func TestQueryLicenseAndByHolder(t *testing.T) {
	f := newTestFixture(t)
	seedLicenseType(t, f, "type.a", true, 0)

	// grant the owner the issue action so they can issue
	grantIssue(t, f, f.OwnerBech, "type.a")

	holderHex := common.HexToAddress("0x2222222222222222222222222222222222222222")
	ids := issueOne(t, f, f.OwnerHex, "type.a", holderHex, "2025-01-01", "")
	require.Len(t, ids, 1)

	// License(id) — no license type needed ----------------------------
	licM := ABI.Methods[LicenseMethod]
	bz, err := f.precompile.License(f.ctx, &licM, []interface{}{ids[0]})
	require.NoError(t, err)

	licOut, err := licM.Outputs.Unpack(bz)
	require.NoError(t, err)
	got := licOut[0].(struct {
		Id        uint64         `json:"id"`
		TypeId    string         `json:"typeId"`
		Holder    common.Address `json:"holder"`
		StartDate string         `json:"startDate"`
		EndDate   string         `json:"endDate"`
		Status    string         `json:"status"`
	})
	require.Equal(t, ids[0], got.Id)
	require.Equal(t, "type.a", got.TypeId)
	require.Equal(t, holderHex, got.Holder)
	require.Equal(t, "active", got.Status)

	// LicensesByHolder(holder) ---------------------------------------
	holderM := ABI.Methods[LicensesByHolderMethod]
	bz, err = f.precompile.LicensesByHolder(f.ctx, &holderM, []interface{}{holderHex})
	require.NoError(t, err)
	holderOut, err := holderM.Outputs.Unpack(bz)
	require.NoError(t, err)
	holderList := holderOut[0].([]struct {
		Id        uint64         `json:"id"`
		TypeId    string         `json:"typeId"`
		Holder    common.Address `json:"holder"`
		StartDate string         `json:"startDate"`
		EndDate   string         `json:"endDate"`
		Status    string         `json:"status"`
	})
	require.Len(t, holderList, 1)
	require.Equal(t, ids[0], holderList[0].Id)

	// LicensesByHolderAndType(holder, typeId) -----------------------
	htM := ABI.Methods[LicensesByHolderAndTypeMethod]
	bz, err = f.precompile.LicensesByHolderAndType(f.ctx, &htM, []interface{}{holderHex, "type.a"})
	require.NoError(t, err)
	htOut, err := htM.Outputs.Unpack(bz)
	require.NoError(t, err)
	htList := htOut[0].([]struct {
		Id        uint64         `json:"id"`
		TypeId    string         `json:"typeId"`
		Holder    common.Address `json:"holder"`
		StartDate string         `json:"startDate"`
		EndDate   string         `json:"endDate"`
		Status    string         `json:"status"`
	})
	require.Len(t, htList, 1)

	// LicensesByType(typeId) -----------------------------------------
	typeM := ABI.Methods[LicensesByTypeMethod]
	bz, err = f.precompile.LicensesByType(f.ctx, &typeM, []interface{}{"type.a"})
	require.NoError(t, err)
	typeOut, err := typeM.Outputs.Unpack(bz)
	require.NoError(t, err)
	typeList := typeOut[0].([]struct {
		Id        uint64         `json:"id"`
		TypeId    string         `json:"typeId"`
		Holder    common.Address `json:"holder"`
		StartDate string         `json:"startDate"`
		EndDate   string         `json:"endDate"`
		Status    string         `json:"status"`
	})
	require.Len(t, typeList, 1)
}

// TestQueryLicenses covers the all-licenses precompile query across types.
func TestQueryLicenses(t *testing.T) {
	f := newTestFixture(t)
	seedLicenseType(t, f, "type.a", true, 0)
	seedLicenseType(t, f, "type.b", true, 0)
	grantIssue(t, f, f.OwnerBech, "type.a")
	grantIssue(t, f, f.OwnerBech, "type.b")

	holderHex := common.HexToAddress("0x2222222222222222222222222222222222222222")
	issueOne(t, f, f.OwnerHex, "type.a", holderHex, "2025-01-01", "")
	issueOne(t, f, f.OwnerHex, "type.b", holderHex, "2025-02-01", "")

	method := ABI.Methods[LicensesMethod]
	bz, err := f.precompile.Licenses(f.ctx, &method, nil)
	require.NoError(t, err)
	out, err := method.Outputs.Unpack(bz)
	require.NoError(t, err)
	list := out[0].([]struct {
		Id        uint64         `json:"id"`
		TypeId    string         `json:"typeId"`
		Holder    common.Address `json:"holder"`
		StartDate string         `json:"startDate"`
		EndDate   string         `json:"endDate"`
		Status    string         `json:"status"`
	})
	require.Len(t, list, 2)
	require.ElementsMatch(t, []string{"type.a", "type.b"}, []string{list[0].TypeId, list[1].TypeId})
}
