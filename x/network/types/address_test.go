package types_test

import (
	"strings"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// TestUppercaseBech32DecodesToSameAccount documents the property that makes
// canonical validation necessary: an all-uppercase bech32 string is a
// different string but the same account. If a future SDK bump starts
// rejecting uppercase outright, this test fails loudly rather than leaving
// the canonical checks looking like dead code.
func TestUppercaseBech32DecodesToSameAccount(t *testing.T) {
	addr := sample.AccAddress()
	upper := strings.ToUpper(addr)
	require.NotEqual(t, addr, upper)

	lowerBz, err := sdk.AccAddressFromBech32(addr)
	require.NoError(t, err)
	upperBz, err := sdk.AccAddressFromBech32(upper)
	require.NoError(t, err, "uppercase bech32 is expected to decode")
	require.Equal(t, lowerBz, upperBz, "both encodings are the same account")
}

func TestValidateCanonicalAddress(t *testing.T) {
	addr := sample.AccAddress()

	tests := []struct {
		name      string
		addr      string
		expErrMsg string
	}{
		{name: "canonical", addr: addr},
		{
			name:      "uppercase alias",
			addr:      strings.ToUpper(addr),
			expErrMsg: "not in canonical form",
		},
		{
			name:      "mixed case",
			addr:      strings.ToUpper(addr[:6]) + addr[6:],
			expErrMsg: "invalid",
		},
		{name: "empty", addr: "", expErrMsg: "invalid"},
		{name: "not bech32", addr: "not-an-address", expErrMsg: "invalid"},
		{
			name:      "hex form",
			addr:      "0x71C7656EC7ab88b098defB751B7401B5f6d8976F",
			expErrMsg: "invalid",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := types.ValidateCanonicalAddress("test", tc.addr)
			if tc.expErrMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.expErrMsg)
		})
	}
}

// TestMsgsRejectNonCanonicalAddresses walks every address field of every msg
// that carries one. The two that matter most are MsgActivateNode.node_address
// and MsgAuthorizeActivationKey.activation_address: neither is the msg's
// signer, so neither is canonicalized by signature verification, and both
// become store keys whose presence is what burns the address.
func TestMsgsRejectNonCanonicalAddresses(t *testing.T) {
	good := sample.AccAddress()
	other := sample.AccAddress()
	bad := strings.ToUpper(good)

	tests := []struct {
		name string
		msg  sdk.HasValidateBasic
	}{
		{"MsgCreateOperatorAccount.admin", &types.MsgCreateOperatorAccount{Admin: bad, Wallet: other}},
		{"MsgCreateOperatorAccount.wallet", &types.MsgCreateOperatorAccount{Admin: other, Wallet: bad}},
		{"MsgCreateNodeType.creator", &types.MsgCreateNodeType{Creator: bad, Id: "t", LicenseTypeId: "l"}},
		{"MsgAuthorizeActivationKey.operator", &types.MsgAuthorizeActivationKey{Operator: bad, ActivationAddress: other}},
		{"MsgAuthorizeActivationKey.activation_address", &types.MsgAuthorizeActivationKey{Operator: other, ActivationAddress: bad}},
		{"MsgDeauthorizeActivationKey.operator", &types.MsgDeauthorizeActivationKey{Operator: bad, ActivationAddress: other}},
		{"MsgDeauthorizeActivationKey.activation_address", &types.MsgDeauthorizeActivationKey{Operator: other, ActivationAddress: bad}},
		{"MsgActivateNode.activation_address", &types.MsgActivateNode{ActivationAddress: bad, Operator: other, NodeAddress: other, NodeType: "t"}},
		{"MsgActivateNode.operator", &types.MsgActivateNode{ActivationAddress: other, Operator: bad, NodeAddress: other, NodeType: "t"}},
		{"MsgActivateNode.node_address", &types.MsgActivateNode{ActivationAddress: other, Operator: other, NodeAddress: bad, NodeType: "t"}},
		{"MsgDeactivateNode.signer", &types.MsgDeactivateNode{Signer: bad, NodeAddress: other}},
		{"MsgDeactivateNode.node_address", &types.MsgDeactivateNode{Signer: other, NodeAddress: bad}},
		{"MsgUpdateNodeStatus.node_address", &types.MsgUpdateNodeStatus{NodeAddress: bad}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.ErrorContains(t, tc.msg.ValidateBasic(), "not in canonical form")
		})
	}
}

// TestGenesisRejectsNonCanonicalAddresses covers the other write path into
// the same stores.
func TestGenesisRejectsNonCanonicalAddresses(t *testing.T) {
	operator := sample.AccAddress()
	keyAddr := sample.AccAddress()
	nodeAddr := sample.AccAddress()
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	base := func() types.GenesisState {
		return types.GenesisState{
			Params: types.DefaultParams(),
			NodeTypes: []types.NodeType{{
				Id: "nodelabs.trust", LicenseTypeId: "nodelabs.node.trust",
			}},
			ActivationKeys: []types.ActivationKey{{
				Address: keyAddr, Operator: operator, Status: types.KeyActive,
				CreatedAt: now,
			}},
			Nodes: []types.Node{{
				Address: nodeAddr, Operator: operator, ActivatedBy: keyAddr,
				Type: "nodelabs.trust", Status: types.NodeActive, LastActiveTime: now,
			}},
		}
	}

	require.NoError(t, base().Validate())

	gs := base()
	gs.ActivationKeys[0].Address = strings.ToUpper(keyAddr)
	gs.Nodes = nil
	require.ErrorContains(t, gs.Validate(), "not in canonical form")

	gs = base()
	gs.Nodes[0].Address = strings.ToUpper(nodeAddr)
	require.ErrorContains(t, gs.Validate(), "not in canonical form")

	gs = base()
	gs.Nodes[0].Operator = strings.ToUpper(operator)
	require.ErrorContains(t, gs.Validate(), "not in canonical form")
}
