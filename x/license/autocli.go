package license

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	modulev1 "github.com/nodelabs-sdk/nodelabs/api/license/v1"
)

func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Query_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the module parameters, including the module owner",
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
					RpcMethod: "GrantsByScope",
					Use:       "grants-by-scope [scope]",
					Short:     "Query the grants that apply to a license type",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "scope"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"action": {Name: "action", Usage: "narrow the result to a single action"},
					},
				},
				{
					RpcMethod: "Can",
					Use:       "can [grantee] [action] [scope]",
					Short:     "Check whether a grantee holds an (action, scope) grant",
					Long:      "Check whether a grantee holds an (action, scope) grant. Pass an empty scope for module-wide actions such as type.create.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "grantee"},
						{ProtoField: "action"},
						{ProtoField: "scope"},
					},
				},
				{
					RpcMethod: "LicenseType",
					Use:       "license-type [id]",
					Short:     "Query a license type by id",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
					},
				},
				{
					RpcMethod: "LicenseTypes",
					Use:       "license-types",
					Short:     "Query all license types",
				},
				{
					RpcMethod: "License",
					Use:       "license [id]",
					Short:     "Query a license by id",
					Long:      "Query a license by id. Ids are unique chain-wide, so no license type is needed.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
					},
				},
				{
					RpcMethod: "Licenses",
					Use:       "licenses",
					Short:     "Query all licenses across all types",
				},
				{
					RpcMethod: "LicensesByType",
					Use:       "licenses-by-type [type-id]",
					Short:     "Query all licenses for a given type",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "type_id"},
					},
				},
				{
					RpcMethod: "LicensesByHolder",
					Use:       "licenses-by-holder [holder]",
					Short:     "Query all licenses held by an address",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "holder"},
					},
				},
				{
					RpcMethod: "LicensesByHolderAndType",
					Use:       "licenses-by-holder-and-type [holder] [type-id]",
					Short:     "Query licenses by holder and type",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "holder"},
						{ProtoField: "type_id"},
					},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              modulev1.Msg_ServiceDesc.ServiceName,
			EnhanceCustomCommand: true,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "CreateLicenseType",
					Use:       "create-license-type [id] [transferrable]",
					Short:     "Create a new license type",
					Long:      "Create a new license type. The signer must hold the module-wide \"type.create\" grant; owning the module is not sufficient on its own. Use --max-supply to limit the number of licenses (default 0 = unlimited).",
					Example:   "nodelabsd tx license create-license-type node.license true --max-supply 1000 --from admin",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "transferrable"},
					},
					FlagOptions: map[string]*autocliv1.FlagOptions{
						"max_supply": {Name: "max-supply", DefaultValue: "0", Usage: "maximum number of licenses that can be issued (0 = unlimited)"},
					},
				},
				{
					RpcMethod: "UpdateLicenseType",
					Use:       "update-license-type [id] [transferrable]",
					Short:     "Update a license type's transferrability",
					Long:      "Update whether licenses of a type can be transferred. Max supply is fixed at creation and cannot be changed.",
					Example:   "nodelabsd tx license update-license-type node.license true --from owner",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "id"},
						{ProtoField: "transferrable"},
					},
				},
				{
					RpcMethod: "RevokeLicenses",
					Use:       "revoke-licenses [license-type-id] [license-ids...]",
					Short:     "Revoke the licenses with the given ids",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "license_type_id"},
						{ProtoField: "license_ids", Varargs: true},
					},
				},
				{
					RpcMethod: "TransferOwnership",
					Use:       "transfer-ownership [new-owner]",
					Short:     "Transfer module ownership to a new address",
					Long:      "Transfer module ownership to a new address. The current owner (--from) must sign. Only the owner parameter changes.",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "new_owner"},
					},
				},
				{
					// Handled by the custom CmdIssueLicenses command; the
					// repeated entries field doesn't map to positional args.
					RpcMethod: "IssueLicenses",
					Skip:      true,
				},
				{
					// Governance-gated; submitted via gov proposal, not the CLI.
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
