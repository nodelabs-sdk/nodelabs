package types_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// TestUppercaseBech32IsTheSameAccount pins the property the guard exists for.
// If a future SDK or bech32 bump starts rejecting uppercase outright, this
// fails and the guard can be reconsidered — but until then, decoding alone
// cannot distinguish the two encodings.
func TestUppercaseBech32IsTheSameAccount(t *testing.T) {
	lower := sample.AccAddress()
	upper := strings.ToUpper(lower)
	require.NotEqual(t, lower, upper)

	lowerBz, err := sdk.AccAddressFromBech32(lower)
	require.NoError(t, err)
	upperBz, err := sdk.AccAddressFromBech32(upper)
	require.NoError(t, err, "uppercase bech32 decodes — which is exactly the hazard")
	require.Equal(t, lowerBz, upperBz, "both encodings are one account")

	// ...but they are two different strings, and grants are keyed on strings.
	require.NotEqual(t, lower, upper)
}

func TestValidateCanonicalAddress(t *testing.T) {
	addr := sample.AccAddress()

	require.NoError(t, types.ValidateCanonicalAddress("grantee", addr))

	// The alias that authenticates as the same account must be rejected.
	err := types.ValidateCanonicalAddress("grantee", strings.ToUpper(addr))
	require.ErrorContains(t, err, "not in canonical form")

	// Plain garbage still fails, with the decode error rather than the
	// canonical-form error.
	err = types.ValidateCanonicalAddress("grantee", "not-an-address")
	require.ErrorContains(t, err, "invalid grantee address")

	err = types.ValidateCanonicalAddress("grantee", "")
	require.ErrorContains(t, err, "invalid grantee address")

	// The field name reaches the message so callers can tell which one failed.
	err = types.ValidateCanonicalAddress("new owner", strings.ToUpper(addr))
	require.ErrorContains(t, err, "new owner address")
}

func TestValidateGrantsRejectsNonCanonicalGrantee(t *testing.T) {
	addr := sample.AccAddress()

	require.NoError(t, types.ValidateGrants([]types.Grant{
		{Grantee: addr, Action: "issue", Scope: "a"},
	}))

	err := types.ValidateGrants([]types.Grant{
		{Grantee: strings.ToUpper(addr), Action: "issue", Scope: "a"},
	})
	require.ErrorContains(t, err, "not in canonical form")

	// Without the guard these two import as distinct grants for one account,
	// and the duplicate check — an exact string comparison — cannot see it.
	err = types.ValidateGrants([]types.Grant{
		{Grantee: addr, Action: "issue", Scope: "a"},
		{Grantee: strings.ToUpper(addr), Action: "issue", Scope: "a"},
	})
	require.Error(t, err)
}
