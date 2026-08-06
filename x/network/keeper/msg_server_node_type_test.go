package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// ---------------------------------------------------------------------------
// CreateNodeType
// ---------------------------------------------------------------------------

// seedLicenseType writes a license type straight into license state. The
// license msg path has its own tests; what matters here is only that the type
// exists for a node type to bind to.
func seedLicenseType(t testing.TB, f *keepertest.NetworkFixture, id string) {
	t.Helper()
	require.NoError(t, f.LicenseKeeper.LicenseTypes.Set(f.Ctx, id, licensetypes.LicenseType{
		Id:           id,
		MaxSupply:    math.ZeroInt(),
		IssuedCount:  math.ZeroInt(),
		ActiveCount:  math.ZeroInt(),
		RevokedCount: math.ZeroInt(),
	}))
}

// TestCreateNodeType covers the happy path: the grant plus an existing license
// type writes the record and its by-license index entry.
func TestCreateNodeType(t *testing.T) {
	f, ms := setup(t)

	creator := sample.AccAddress()
	f.GrantNetwork(t, creator, types.ActionNodeTypeCreate)
	seedLicenseType(t, f, "nodelabs.node.gpu")

	_, err := ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.gpu",
	})
	require.NoError(t, err)

	// The signing address is not part of the record: the grant is the whole
	// authorization, so nothing about the signer is retained.
	nt, err := f.Keeper.NodeTypes.Get(f.Ctx, "nodelabs.gpu")
	require.NoError(t, err)
	require.Equal(t, types.NodeType{
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.gpu",
	}, nt)

	// The inverse mapping is what makes "which node type does this license type
	// back" a single read, so its entry is part of the contract.
	bound, err := f.Keeper.NodeTypeByLicenseType.Get(f.Ctx, "nodelabs.node.gpu")
	require.NoError(t, err)
	require.Equal(t, "nodelabs.gpu", bound)
}

// TestCreateNodeTypeRequiresGrant: an existing license type is not enough. The
// grant is the only authorization for registering node types, so without it
// nothing is written.
func TestCreateNodeTypeRequiresGrant(t *testing.T) {
	f, ms := setup(t)

	creator := sample.AccAddress()
	seedLicenseType(t, f, "nodelabs.node.gpu")

	msg := &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.gpu",
	}

	_, err := ms.CreateNodeType(f.Ctx, msg)
	require.ErrorIs(t, err, types.ErrUnauthorized)
	require.ErrorContains(t, err, types.ActionNodeTypeCreate)

	// Nothing was written on the rejected attempt.
	exists, err := f.Keeper.NodeTypes.Has(f.Ctx, "nodelabs.gpu")
	require.NoError(t, err)
	require.False(t, exists)

	f.GrantNetwork(t, creator, types.ActionNodeTypeCreate)
	_, err = ms.CreateNodeType(f.Ctx, msg)
	require.NoError(t, err)
}

// TestCreateNodeTypeGrantIsTheOnlyGate: the grant authorizes binding against
// any existing license type, including one created by somebody else. The
// license type carries no record of who created it, so there is nothing left
// for the handler to compare the signer against — this pins that the signer's
// relationship to the license type is deliberately not consulted.
func TestCreateNodeTypeGrantIsTheOnlyGate(t *testing.T) {
	f, ms := setup(t)

	// The license type records no address at all, so the handler has nothing to
	// compare the signer against: any grantee may bind against it.
	seedLicenseType(t, f, "nodelabs.node.gpu")

	stranger := sample.AccAddress()
	f.GrantNetwork(t, stranger, types.ActionNodeTypeCreate)

	_, err := ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       stranger,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.gpu",
	})
	require.NoError(t, err)

	nt, err := f.Keeper.NodeTypes.Get(f.Ctx, "nodelabs.gpu")
	require.NoError(t, err)
	require.Equal(t, "nodelabs.node.gpu", nt.LicenseTypeId)
}

// TestCreateNodeTypeUnknownLicenseType: a binding to a license type that does
// not exist is a distinct failure from an authorization one.
func TestCreateNodeTypeUnknownLicenseType(t *testing.T) {
	f, ms := setup(t)

	creator := sample.AccAddress()
	f.GrantNetwork(t, creator, types.ActionNodeTypeCreate)

	_, err := ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "does.not.exist",
	})
	require.ErrorIs(t, err, types.ErrLicenseTypeNotFound)
}

// TestCreateNodeTypeDuplicate: ids are single-use. A second registration must
// not silently repoint an existing type's license binding, because nodes
// already carry that type.
func TestCreateNodeTypeDuplicate(t *testing.T) {
	f, ms := setup(t)

	creator := sample.AccAddress()
	f.GrantNetwork(t, creator, types.ActionNodeTypeCreate)
	seedLicenseType(t, f, "nodelabs.node.gpu")
	seedLicenseType(t, f, "nodelabs.node.other")

	_, err := ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.gpu",
	})
	require.NoError(t, err)

	// Same id, different license type, same (authorized) signer.
	_, err = ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.other",
	})
	require.ErrorIs(t, err, types.ErrNodeTypeExists)

	nt, err := f.Keeper.NodeTypes.Get(f.Ctx, "nodelabs.gpu")
	require.NoError(t, err)
	require.Equal(t, "nodelabs.node.gpu", nt.LicenseTypeId, "the original binding must survive")
}

// TestCreateNodeTypeLicenseTypeAlreadyBound: the binding is one-to-one in both
// directions. A license type already backing a node type cannot back a second,
// even from the same signer — otherwise the two would collide in the inverse
// mapping and one would silently win.
func TestCreateNodeTypeLicenseTypeAlreadyBound(t *testing.T) {
	f, ms := setup(t)

	creator := sample.AccAddress()
	f.GrantNetwork(t, creator, types.ActionNodeTypeCreate)
	seedLicenseType(t, f, "nodelabs.node.gpu")

	_, err := ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.gpu",
		LicenseTypeId: "nodelabs.node.gpu",
	})
	require.NoError(t, err)

	_, err = ms.CreateNodeType(f.Ctx, &types.MsgCreateNodeType{
		Creator:       creator,
		Id:            "nodelabs.second",
		LicenseTypeId: "nodelabs.node.gpu",
	})
	require.ErrorIs(t, err, types.ErrLicenseTypeBound)
	require.ErrorContains(t, err, "nodelabs.gpu")

	// The rejected node type was not written, and the original binding stands.
	exists, err := f.Keeper.NodeTypes.Has(f.Ctx, "nodelabs.second")
	require.NoError(t, err)
	require.False(t, exists)

	bound, err := f.Keeper.NodeTypeByLicenseType.Get(f.Ctx, "nodelabs.node.gpu")
	require.NoError(t, err)
	require.Equal(t, "nodelabs.gpu", bound)
}

// TestCreateNodeTypeValidateBasic pins the stateless checks.
func TestCreateNodeTypeValidateBasic(t *testing.T) {
	creator := sample.AccAddress()

	require.NoError(t, (&types.MsgCreateNodeType{
		Creator: creator, Id: "t", LicenseTypeId: "l",
	}).ValidateBasic())

	err := (&types.MsgCreateNodeType{Creator: creator, LicenseTypeId: "l"}).ValidateBasic()
	require.ErrorIs(t, err, types.ErrInvalidNodeType)

	err = (&types.MsgCreateNodeType{Creator: creator, Id: "t"}).ValidateBasic()
	require.ErrorIs(t, err, types.ErrLicenseTypeNotFound)
}
