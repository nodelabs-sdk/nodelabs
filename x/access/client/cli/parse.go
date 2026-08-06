// Package cli holds the argument grammar shared by every module's
// grant-access and revoke-access commands, so the boundary syntax is identical
// no matter which module a grant is filed under.
package cli

import (
	"fmt"
	"strings"

	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// ModuleWideScopes is the scopes argument requesting a module-wide grant: an
// empty scope list. It is used both by modules that do not scope their actions
// at all and for the individual actions a module declares module-wide, which
// reject a non-empty scope.
const ModuleWideScopes = "-"

// ParseGrantEntries parses the grant-access argument pair into message
// entries. actionsArg is a comma-delimited action list; scopesArg is a
// comma-delimited scope list, or ModuleWideScopes.
//
// One entry is produced per action, each covering all the listed scopes.
// Granting a mix of scoped and module-wide actions therefore takes two
// invocations — one with scopes, one with "-".
func ParseGrantEntries(actionsArg, scopesArg string) ([]types.ActionScopes, error) {
	var scopes []string
	if strings.TrimSpace(scopesArg) != ModuleWideScopes {
		scopes = strings.Split(scopesArg, ",")
		for i, s := range scopes {
			scopes[i] = strings.TrimSpace(s)
		}
	}

	actions := strings.Split(actionsArg, ",")
	entries := make([]types.ActionScopes, 0, len(actions))
	for _, action := range actions {
		action = strings.TrimSpace(action)
		if action == "" {
			continue
		}
		entries = append(entries, types.ActionScopes{
			Action: action,
			Scopes: scopes,
		})
	}

	if len(entries) == 0 {
		return nil, fmt.Errorf("at least one action must be specified")
	}
	return entries, nil
}

// ParseRevokePairs parses the revoke-access arguments into (action, scope)
// pairs. Each argument is colon-delimited "action:scope"; a bare action with
// no colon targets the module-wide (empty scope) grant.
func ParseRevokePairs(args []string) ([]types.ActionScope, error) {
	pairs := make([]types.ActionScope, 0, len(args))
	for i, arg := range args {
		parts := strings.SplitN(arg, ":", 2)
		action := strings.TrimSpace(parts[0])
		if action == "" {
			return nil, fmt.Errorf("pair %d: action must be non-empty (got %q)", i, arg)
		}
		var scope string
		if len(parts) == 2 {
			scope = strings.TrimSpace(parts[1])
		}
		pairs = append(pairs, types.ActionScope{Action: action, Scope: scope})
	}

	if len(pairs) == 0 {
		return nil, fmt.Errorf("at least one action must be specified")
	}
	return pairs, nil
}
