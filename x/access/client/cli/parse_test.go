package cli_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/x/access/client/cli"
	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func TestParseGrantEntries(t *testing.T) {
	tests := []struct {
		name       string
		actions    string
		scopes     string
		want       []types.ActionScopes
		wantErrStr string
	}{
		{
			name:    "one action one scope",
			actions: "issue",
			scopes:  "node.license",
			want:    []types.ActionScopes{{Action: "issue", Scopes: []string{"node.license"}}},
		},
		{
			name:    "each action covers every scope",
			actions: "issue,revoke",
			scopes:  "node.license,validator.license",
			want: []types.ActionScopes{
				{Action: "issue", Scopes: []string{"node.license", "validator.license"}},
				{Action: "revoke", Scopes: []string{"node.license", "validator.license"}},
			},
		},
		{
			name:    "dash requests a module-wide grant",
			actions: "type.create",
			scopes:  cli.ModuleWideScopes,
			want:    []types.ActionScopes{{Action: "type.create", Scopes: nil}},
		},
		{
			name:    "surrounding whitespace is trimmed",
			actions: " issue , revoke ",
			scopes:  " a , b ",
			want: []types.ActionScopes{
				{Action: "issue", Scopes: []string{"a", "b"}},
				{Action: "revoke", Scopes: []string{"a", "b"}},
			},
		},
		{
			name:    "empty action entries are skipped",
			actions: "issue,,revoke",
			scopes:  "a",
			want: []types.ActionScopes{
				{Action: "issue", Scopes: []string{"a"}},
				{Action: "revoke", Scopes: []string{"a"}},
			},
		},
		{
			name:       "no actions",
			actions:    "",
			scopes:     "a",
			wantErrStr: "at least one action",
		},
		{
			name:       "only separators",
			actions:    ",,",
			scopes:     "a",
			wantErrStr: "at least one action",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cli.ParseGrantEntries(tc.actions, tc.scopes)
			if tc.wantErrStr != "" {
				require.ErrorContains(t, err, tc.wantErrStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestParseRevokePairs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		want       []types.ActionScope
		wantErrStr string
	}{
		{
			name: "action and scope",
			args: []string{"issue:node.license"},
			want: []types.ActionScope{{Action: "issue", Scope: "node.license"}},
		},
		{
			name: "bare action targets the module-wide grant",
			args: []string{"type.create"},
			want: []types.ActionScope{{Action: "type.create", Scope: ""}},
		},
		{
			name: "trailing colon also targets the module-wide grant",
			args: []string{"type.create:"},
			want: []types.ActionScope{{Action: "type.create", Scope: ""}},
		},
		{
			name: "scopes may contain colons",
			args: []string{"issue:ns:sub"},
			want: []types.ActionScope{{Action: "issue", Scope: "ns:sub"}},
		},
		{
			name: "several pairs",
			args: []string{"issue:a", "revoke:b"},
			want: []types.ActionScope{
				{Action: "issue", Scope: "a"},
				{Action: "revoke", Scope: "b"},
			},
		},
		{
			name:       "empty action",
			args:       []string{":a"},
			wantErrStr: "pair 0: action must be non-empty",
		},
		{
			name:       "no pairs",
			args:       nil,
			wantErrStr: "at least one action",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := cli.ParseRevokePairs(tc.args)
			if tc.wantErrStr != "" {
				require.ErrorContains(t, err, tc.wantErrStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// The CLI grammar is why ValidateAction excludes ',' and ':': an action
// containing either would be indistinguishable from a delimiter.
func TestParsedActionsSurviveValidation(t *testing.T) {
	entries, err := cli.ParseGrantEntries("issue,type.create", cli.ModuleWideScopes)
	require.NoError(t, err)
	require.NoError(t, types.ValidateGrantEntries(entries))

	pairs, err := cli.ParseRevokePairs([]string{"issue:a", "type.create"})
	require.NoError(t, err)
	require.NoError(t, types.ValidateRevokePairs(pairs))
}
