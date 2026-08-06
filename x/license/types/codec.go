package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateLicenseType{}, "nodelabs/x/license/MsgCreateLicenseType", nil)
	cdc.RegisterConcrete(&MsgIssueLicenses{}, "nodelabs/x/license/MsgIssueLicenses", nil)
	cdc.RegisterConcrete(&MsgRevokeLicenses{}, "nodelabs/x/license/MsgRevokeLicenses", nil)
	cdc.RegisterConcrete(&MsgUpdateLicenseType{}, "nodelabs/x/license/MsgUpdateLicenseType", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "nodelabs/x/license/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgTransferOwnership{}, "nodelabs/x/license/MsgTransferOwnership", nil)
	cdc.RegisterConcrete(&MsgGrantAccess{}, "nodelabs/x/license/MsgGrantAccess", nil)
	cdc.RegisterConcrete(&MsgRevokeAccess{}, "nodelabs/x/license/MsgRevokeAccess", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateLicenseType{},
		&MsgIssueLicenses{},
		&MsgRevokeLicenses{},
		&MsgUpdateLicenseType{},
		&MsgUpdateParams{},
		&MsgTransferOwnership{},
		&MsgGrantAccess{},
		&MsgRevokeAccess{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
