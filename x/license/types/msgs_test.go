package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/x/tx/signing"
	"github.com/cosmos/cosmos-sdk/codec/address"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/gogoproto/proto"

	licensev1 "github.com/nodelabs-sdk/nodelabs/api/license/v1"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// TestMsgSignerAnnotations resolves signers the way the SDK does at tx
// handling time, from the cosmos.msg.v1.signer proto annotation rather than
// the Go struct. A rename that leaves the annotation pointing at a field that
// no longer exists still compiles, so this is the check that catches it.
func TestMsgSignerAnnotations(t *testing.T) {
	prefix := sdk.GetConfig().GetBech32AccountAddrPrefix()
	registry, err := codectypes.NewInterfaceRegistryWithOptions(codectypes.InterfaceRegistryOptions{
		ProtoFiles: proto.HybridResolver,
		SigningOptions: signing.Options{
			AddressCodec:          address.NewBech32Codec(prefix),
			ValidatorAddressCodec: address.NewBech32Codec(prefix + "valoper"),
		},
	})
	require.NoError(t, err)
	types.RegisterInterfaces(registry)

	addr := sample.AccAddress()
	want, err := sdk.AccAddressFromBech32(addr)
	require.NoError(t, err)

	// CreateLicenseType signs as "creator": the holder of type.create, which
	// is not necessarily the module owner.
	signers, err := registry.SigningContext().GetSigners(&licensev1.MsgCreateLicenseType{Creator: addr})
	require.NoError(t, err)
	require.Equal(t, [][]byte{want}, signers)

	// UpdateLicenseType still signs as "owner" — it remains owner-gated.
	signers, err = registry.SigningContext().GetSigners(&licensev1.MsgUpdateLicenseType{Owner: addr})
	require.NoError(t, err)
	require.Equal(t, [][]byte{want}, signers)

	// UpdateParams signs as the governance authority.
	signers, err = registry.SigningContext().GetSigners(&licensev1.MsgUpdateParams{Authority: addr})
	require.NoError(t, err)
	require.Equal(t, [][]byte{want}, signers)

	// The ownership and grant messages all sign as the current owner.
	signers, err = registry.SigningContext().GetSigners(&licensev1.MsgTransferOwnership{Owner: addr})
	require.NoError(t, err)
	require.Equal(t, [][]byte{want}, signers)

	signers, err = registry.SigningContext().GetSigners(&licensev1.MsgGrantAccess{Owner: addr})
	require.NoError(t, err)
	require.Equal(t, [][]byte{want}, signers)

	signers, err = registry.SigningContext().GetSigners(&licensev1.MsgRevokeAccess{Owner: addr})
	require.NoError(t, err)
	require.Equal(t, [][]byte{want}, signers)
}

func TestAccessMsgValidateBasic(t *testing.T) {
	owner := sample.AccAddress()
	grantee := sample.AccAddress()

	require.NoError(t, (&types.MsgUpdateParams{Authority: owner, Params: types.Params{Owner: grantee}}).ValidateBasic())
	require.NoError(t, (&types.MsgUpdateParams{Authority: owner, Params: types.Params{}}).ValidateBasic())
	require.Error(t, (&types.MsgUpdateParams{Authority: "bad"}).ValidateBasic())
	require.ErrorContains(t,
		(&types.MsgUpdateParams{Authority: owner, Params: types.Params{Owner: "bad"}}).ValidateBasic(),
		"invalid owner address")

	require.NoError(t, (&types.MsgTransferOwnership{Owner: owner, NewOwner: grantee}).ValidateBasic())
	require.Error(t, (&types.MsgTransferOwnership{Owner: "bad", NewOwner: grantee}).ValidateBasic())
	require.Error(t, (&types.MsgTransferOwnership{Owner: owner, NewOwner: "bad"}).ValidateBasic())

	require.NoError(t, (&types.MsgGrantAccess{
		Owner:   owner,
		Grantee: grantee,
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionIssue, Scopes: []string{"lt1"}}},
	}).ValidateBasic())
	require.ErrorContains(t, (&types.MsgGrantAccess{
		Owner:   owner,
		Grantee: grantee,
	}).ValidateBasic(), "must not be empty")
	require.Error(t, (&types.MsgGrantAccess{
		Owner:   owner,
		Grantee: "bad",
		Grants:  []accesstypes.ActionScopes{{Action: types.ActionIssue}},
	}).ValidateBasic())

	require.NoError(t, (&types.MsgRevokeAccess{
		Owner:   owner,
		Grantee: grantee,
		Actions: []accesstypes.ActionScope{{Action: types.ActionIssue, Scope: "lt1"}},
	}).ValidateBasic())
	require.ErrorContains(t, (&types.MsgRevokeAccess{
		Owner:   owner,
		Grantee: grantee,
	}).ValidateBasic(), "must not be empty")
}

func TestMsgUpdateLicenseTypeValidateBasic(t *testing.T) {
	owner := sample.AccAddress()

	require.NoError(t, (&types.MsgUpdateLicenseType{Owner: owner, Id: "lt1"}).ValidateBasic())
	require.NoError(t, (&types.MsgUpdateLicenseType{Owner: owner, Id: "lt1", Transferrable: true}).ValidateBasic())

	require.ErrorContains(t,
		(&types.MsgUpdateLicenseType{Owner: "x", Id: "lt1"}).ValidateBasic(),
		"invalid owner address")
	require.ErrorContains(t,
		(&types.MsgUpdateLicenseType{Owner: owner}).ValidateBasic(),
		"license type id cannot be empty")
}

func TestMsgRevokeLicensesValidateBasic(t *testing.T) {
	revoker := sample.AccAddress()

	require.NoError(t, (&types.MsgRevokeLicenses{
		Revoker: revoker, LicenseTypeId: "lt1", LicenseIds: []uint64{1, 2, 3},
	}).ValidateBasic())

	require.ErrorContains(t,
		(&types.MsgRevokeLicenses{Revoker: "bad", LicenseTypeId: "lt1", LicenseIds: []uint64{1}}).ValidateBasic(),
		"invalid revoker address")
	require.ErrorContains(t,
		(&types.MsgRevokeLicenses{Revoker: revoker, LicenseIds: []uint64{1}}).ValidateBasic(),
		"license type id cannot be empty")
	require.ErrorContains(t,
		(&types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "lt1"}).ValidateBasic(),
		"license ids cannot be empty")
	require.ErrorContains(t,
		(&types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "lt1", LicenseIds: []uint64{1, 2, 1}}).ValidateBasic(),
		"duplicate license id")
	require.ErrorContains(t,
		(&types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "lt1", LicenseIds: []uint64{0}}).ValidateBasic(),
		"not a valid id")

	tooMany := make([]uint64, types.MaxRevokeBatchSize+1)
	for i := range tooMany {
		tooMany[i] = uint64(i) + types.FirstLicenseID
	}
	require.ErrorContains(t,
		(&types.MsgRevokeLicenses{Revoker: revoker, LicenseTypeId: "lt1", LicenseIds: tooMany}).ValidateBasic(),
		"exceeds max batch size")
}
