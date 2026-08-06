package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgCreateOperatorAccount{}, "nodelabs/x/network/MsgCreateOperatorAccount", nil)
	cdc.RegisterConcrete(&MsgCreateNodeType{}, "nodelabs/x/network/MsgCreateNodeType", nil)
	cdc.RegisterConcrete(&MsgAuthorizeActivationKey{}, "nodelabs/x/network/MsgAuthorizeActivationKey", nil)
	cdc.RegisterConcrete(&MsgDeauthorizeActivationKey{}, "nodelabs/x/network/MsgDeauthorizeActivationKey", nil)
	cdc.RegisterConcrete(&MsgActivateNode{}, "nodelabs/x/network/MsgActivateNode", nil)
	cdc.RegisterConcrete(&MsgDeactivateNode{}, "nodelabs/x/network/MsgDeactivateNode", nil)
	cdc.RegisterConcrete(&MsgUpdateNodeStatus{}, "nodelabs/x/network/MsgUpdateNodeStatus", nil)
	cdc.RegisterConcrete(&MsgUpdateParams{}, "nodelabs/x/network/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgTransferOwnership{}, "nodelabs/x/network/MsgTransferOwnership", nil)
	cdc.RegisterConcrete(&MsgGrantAccess{}, "nodelabs/x/network/MsgGrantAccess", nil)
	cdc.RegisterConcrete(&MsgRevokeAccess{}, "nodelabs/x/network/MsgRevokeAccess", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations((*sdk.Msg)(nil),
		&MsgCreateOperatorAccount{},
		&MsgCreateNodeType{},
		&MsgAuthorizeActivationKey{},
		&MsgDeauthorizeActivationKey{},
		&MsgActivateNode{},
		&MsgDeactivateNode{},
		&MsgUpdateNodeStatus{},
		&MsgUpdateParams{},
		&MsgTransferOwnership{},
		&MsgGrantAccess{},
		&MsgRevokeAccess{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
