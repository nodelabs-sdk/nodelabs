package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

	accesscli "github.com/nodelabs-sdk/nodelabs/x/access/client/cli"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// GetTxCmd returns the transaction commands for this module.
func GetTxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      fmt.Sprintf("%s transactions subcommands", types.ModuleName),
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	cmd.AddCommand(CmdIssueLicenses())
	cmd.AddCommand(CmdRevokeLicenses())
	cmd.AddCommand(CmdGrantAccess())
	cmd.AddCommand(CmdRevokeAccess())

	return cmd
}

// CmdIssueLicenses returns a command to issue licenses to one or more holders,
// across one or more license types, in a single transaction.
//
// Usage:
//
//	issue-licenses [entry1] [entry2] ...
//
// Each entry is colon-delimited: license_type_id:holder:count:start_date[:end_date]
// The end_date is optional (omit or leave empty after the fourth colon).
// The issuer is taken from --from.
func CmdIssueLicenses() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "issue-licenses [entries...]",
		Short: "Issue licenses to one or more holders in a single transaction",
		Long: `Issue licenses in a single transaction. Each entry can target a different
license type and holder. The issuer (--from) must hold the "issue" grant for
every referenced license type.

Each entry is colon-delimited:
  license_type_id:holder:count:start_date[:end_date]

The end_date is optional. If omitted, the license has no expiry.

Example:
  nodelabsd tx license issue-licenses \
    node.license:nodelabs1abc...:1:2025-01-01:2026-01-01 \
    validator.license:nodelabs1def...:3:2025-01-01 \
    --from admin --gas auto --fees 100000aatom -y`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			entries := make([]types.IssueLicenseEntry, 0, len(args))
			for i, arg := range args {
				parts := strings.SplitN(arg, ":", 5)
				if len(parts) < 4 {
					return fmt.Errorf("entry %d: expected format license_type_id:holder:count:start_date[:end_date], got %q", i, arg)
				}

				licenseTypeID := strings.TrimSpace(parts[0])
				if licenseTypeID == "" {
					return fmt.Errorf("entry %d: license type id must not be empty", i)
				}

				holder := strings.TrimSpace(parts[1])
				if _, err := sdk.AccAddressFromBech32(holder); err != nil {
					return fmt.Errorf("entry %d: invalid holder address %q: %w", i, holder, err)
				}

				count, err := strconv.ParseUint(strings.TrimSpace(parts[2]), 10, 64)
				if err != nil {
					return fmt.Errorf("entry %d: invalid count %q: %w", i, parts[2], err)
				}

				startDate := strings.TrimSpace(parts[3])
				var endDate string
				if len(parts) == 5 {
					endDate = strings.TrimSpace(parts[4])
				}

				entries = append(entries, types.IssueLicenseEntry{
					LicenseTypeId: licenseTypeID,
					Holder:        holder,
					Count:         count,
					StartDate:     startDate,
					EndDate:       endDate,
				})
			}

			msg := &types.MsgIssueLicenses{
				Issuer:  clientCtx.GetFromAddress().String(),
				Entries: entries,
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// CmdRevokeLicenses returns a command to revoke licenses by id.
//
// Usage:
//
//	revoke-licenses [license-type-id] [license-ids]
//
// Where [license-ids] is a comma-delimited list of license ids. The revoker
// is taken from --from.
func CmdRevokeLicenses() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-licenses [license-type-id] [license-ids]",
		Short: "Revoke the licenses with the given ids",
		Long: `Revoke licenses by id. The revoker (--from) must hold the "revoke" grant for the license type.

[license-ids] is a comma-delimited list of license ids. Each id must be an active license
of the given type. Their status is set to "revoked" and end_date is set to the current
block date.

Example:
  nodelabsd tx license revoke-licenses node.license 12,13,27 \
    --from admin --gas auto --fees 100000aatom -y`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			rawIDs := strings.Split(args[1], ",")
			ids := make([]uint64, 0, len(rawIDs))
			for _, raw := range rawIDs {
				id, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
				if err != nil {
					return fmt.Errorf("invalid license id %q: %w", raw, err)
				}
				ids = append(ids, id)
			}

			msg := &types.MsgRevokeLicenses{
				Revoker:       clientCtx.GetFromAddress().String(),
				LicenseTypeId: args[0],
				LicenseIds:    ids,
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// CmdGrantAccess returns a command to grant actions to an address.
//
// Usage:
//
//	grant-access [grantee] [actions] [scopes]
//
// Where [actions] is a comma-delimited list (e.g. "issue,revoke") and [scopes]
// is a comma-delimited list of license type ids, or "-" for a module-wide
// grant. One entry is created per action, each sharing the same scopes. The
// module owner is taken from --from.
func CmdGrantAccess() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "grant-access [grantee] [actions] [scopes]",
		Short: "Grant actions to an address",
		Long: `Grant actions to a given address. The module owner (--from) must sign.

[grantee]  The address receiving the grants.
[actions]  Comma-delimited list of actions to grant. Query the vocabulary with:
           nodelabsd query license actions
[scopes]   Comma-delimited list of license type ids these actions apply to, or
           "-" for a module-wide grant. Module-wide actions (see the "unscoped"
           field of the actions query) reject a non-empty scope.

One entry is created per action, each covering all specified scopes. Granting a
mix of scoped and module-wide actions therefore takes two invocations — one
with scopes, one with "-".

Grants are MERGED with any existing grants for the grantee — previously granted
actions and scopes are preserved. To remove specific (action, scope) pairs, use
revoke-access.

Example:
  nodelabsd tx license grant-access nodelabs1abc... issue,revoke node.license,validator.license \
    --from owner --gas auto --gas-adjustment 1.5 --fees 100000aatom -y`,
		Args: cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			grantee := args[0]
			if _, err := sdk.AccAddressFromBech32(grantee); err != nil {
				return fmt.Errorf("invalid grantee address %q: %w", grantee, err)
			}

			entries, err := accesscli.ParseGrantEntries(args[1], args[2])
			if err != nil {
				return err
			}

			msg := &types.MsgGrantAccess{
				Owner:   clientCtx.GetFromAddress().String(),
				Grantee: grantee,
				Grants:  entries,
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// CmdRevokeAccess returns a command to remove specific (action, scope) pairs
// from a grantee.
//
// Usage:
//
//	revoke-access [grantee] [pair1] [pair2] ...
//
// Each pair is colon-delimited: action:scope. A trailing colon (or a bare
// action with no colon) targets the module-wide grant. Pairs that aren't
// currently granted are silently ignored. The module owner is taken from
// --from.
func CmdRevokeAccess() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "revoke-access [grantee] [action:scope ...]",
		Short: "Revoke specific (action, scope) pairs from a grantee",
		Long: `Revoke specific (action, scope) pairs from an address. The module owner
(--from) must sign.

Each pair after the grantee is colon-delimited:
  action:scope

A bare action with no colon targets the module-wide (empty scope) grant.

Pairs that aren't currently granted are silently ignored.

Example:
  nodelabsd tx license revoke-access nodelabs1abc... \
    issue:node.license revoke:validator.license \
    --from owner --gas auto --fees 100000aatom -y`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			grantee := args[0]
			if _, err := sdk.AccAddressFromBech32(grantee); err != nil {
				return fmt.Errorf("invalid grantee address %q: %w", grantee, err)
			}

			pairs, err := accesscli.ParseRevokePairs(args[1:])
			if err != nil {
				return err
			}

			msg := &types.MsgRevokeAccess{
				Owner:   clientCtx.GetFromAddress().String(),
				Grantee: grantee,
				Actions: pairs,
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
