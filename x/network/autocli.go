package network

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	modulev1 "github.com/nodelabs-sdk/nodelabs/api/network/v1"
)

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Query_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the network module parameters, including the module owner",
				},
				{
					RpcMethod: "Actions",
					Use:       "actions",
					Short:     "Query the action vocabulary this module recognises",
				},
				{
					RpcMethod: "Grants",
					Use:       "grants",
					Short:     "Query every access grant in the module",
				},
				{
					RpcMethod: "GrantsByGrantee",
					Use:       "grants-by-grantee [grantee]",
					Short:     "Query the grants held by an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "grantee"},
					},
				},
				{
					RpcMethod: "Can",
					Use:       "can [grantee] [action]",
					Short:     "Check whether a grantee holds an action grant",
					Long:      "Check whether a grantee holds an action grant. This module does not scope its actions, so no scope is taken.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "grantee"},
						{ProtoField: "action"},
					},
				},
				{
					RpcMethod: "Node",
					Use:       "node [address]",
					Short:     "Query a node by address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
					},
				},
				{
					RpcMethod: "Nodes",
					Use:       "nodes",
					Short:     "Query all nodes",
					Long:      "Query all nodes. Node records are retained after deactivation, so this returns every node ever activated unless --status is given.",
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"status": {Name: "status", Usage: "filter by node status: active or deactivated (default: all statuses)"},
					},
				},
				{
					RpcMethod: "NodesByOperator",
					Use:       "nodes-by-operator [operator]",
					Short:     "Query all nodes activated under an operator",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "operator"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"status": {Name: "status", Usage: "filter by node status: active or deactivated (default: all statuses)"},
					},
				},
				{
					RpcMethod: "NodeType",
					Use:       "node-type [id]",
					Short:     "Query a registered node type by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
					},
				},
				{
					RpcMethod: "NodeTypes",
					Use:       "node-types",
					Short:     "Query registered node types",
					Long:      "Query registered node types. Pass --license-type-id to list only the node types bound to one license type.",
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"license_type_id": {Name: "license-type-id", Usage: "list only the node types bound to this license type (default: all node types)"},
					},
				},
				{
					RpcMethod: "ActivationKey",
					Use:       "activation-key [address]",
					Short:     "Query an activation key by address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "address"},
					},
				},
				{
					RpcMethod: "ActivationKeys",
					Use:       "activation-keys [operator]",
					Short:     "Query the activation keys authorized by an operator",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "operator"},
					},
				},
				{
					RpcMethod: "NodeCounts",
					Use:       "node-counts [operator]",
					Short:     "Query an operator's per-node-type tallies and activation limits",
					Long:      "Query an operator's node tallies and activation limits, broken down by node type. Limits come from the operator's licenses of the license type each node type is bound to, so they are independent per type. Pass --node-type to restrict the result to one.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "operator"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"node_type": {Name: "node-type", Usage: "restrict the result to one node type (default: every registered node type)"},
					},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              modulev1.Msg_ServiceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "CreateOperatorAccount",
					Use:       "create-operator-account [wallet]",
					Short:     "Create an on-chain account for an operator wallet",
					Long:      "Create an on-chain account for an operator wallet. The signer must hold the network module's wallet.create grant.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "wallet"},
					},
				},
				{
					RpcMethod: "CreateNodeType",
					Use:       "create-node-type [id] [license-type-id]",
					Short:     "Register a node type bound to a license type",
					Long:      "Register a node type bound to a license type. The signer must hold the network module's nodetype.create grant, and the named license type must already exist. Node types are permanent: the id and the binding cannot be changed or removed.",
					Example:   "nodelabsd tx network create-node-type nodelabs.trust nodelabs.node --from admin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "license_type_id"},
					},
				},
				{
					RpcMethod: "AuthorizeActivationKey",
					Use:       "authorize-activation-key [activation-address]",
					Short:     "Authorize an activation key for the signing operator wallet",
					Example:   "nodelabsd tx network authorize-activation-key cosmos1... --from operator",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "activation_address"},
					},
				},
				{
					RpcMethod: "DeauthorizeActivationKey",
					Use:       "deauthorize-activation-key [activation-address]",
					Short:     "Disable an activation key (irreversible; the address stays bound)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "activation_address"},
					},
				},
				{
					RpcMethod: "ActivateNode",
					Use:       "activate-node [operator] [node-address] [node-type]",
					Short:     "Activate a node under an operator, signed by an activation key",
					Long:      "Activate a node under an operator, signed by an activation key. The node type must already be registered with create-node-type.",
					Example:   "nodelabsd tx network activate-node cosmos1op... cosmos1node... nodelabs.trust --from activation-key",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "operator"},
						{ProtoField: "node_address"},
						{ProtoField: "node_type"},
					},
				},
				{
					RpcMethod: "DeactivateNode",
					Use:       "deactivate-node [node-address]",
					Short:     "Deactivate a node (irreversible)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "node_address"},
					},
				},
				{
					// The nested payload doesn't map to positional args; node
					// software submits this msg programmatically.
					RpcMethod: "UpdateNodeStatus",
					Skip:      true,
				},
				{
					RpcMethod: "TransferOwnership",
					Use:       "transfer-ownership [new-owner]",
					Short:     "Transfer module ownership to a new address",
					Long:      "Transfer module ownership to a new address. The current owner (--from) must sign. Only the owner parameter changes; every other parameter is preserved.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "new_owner"},
					},
				},
				{
					// Gov-gated; submitted via governance proposal.
					RpcMethod: "UpdateParams",
					Skip:      true,
				},
				{
					// Handled by the custom CmdGrantAccess command; the
					// repeated grants field doesn't map to positional args.
					RpcMethod: "GrantAccess",
					Skip:      true,
				},
				{
					// Handled by the custom CmdRevokeAccess command; the
					// repeated actions field doesn't map to positional args.
					RpcMethod: "RevokeAccess",
					Skip:      true,
				},
			},
		},
	}
}
