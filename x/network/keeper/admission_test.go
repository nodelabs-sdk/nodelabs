package keeper_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// TestAdmissionAuthorize: standing requires a licensed operator; the daily
// counter burns per attempt and resets across UTC days.
func TestAdmissionAuthorize(t *testing.T) {
	f, _ := setup(t)
	operator := sample.AccAddress()
	msg := &types.MsgAuthorizeActivationKey{Operator: operator, ActivationAddress: sample.AccAddress()}

	// A licenseless address has no free tx channel.
	require.ErrorIs(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg), types.ErrNoActiveLicenses)

	f.IssueLicenses(t, operator, 1)

	params, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)

	// Every attempt burns quota — including ones whose handler would fail.
	for i := uint64(0); i < params.RecentKeyLimit; i++ {
		require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg))
	}
	require.ErrorIs(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg), types.ErrQuotaExceeded)

	// The counter resets on the next UTC day.
	nextDay := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(24 * time.Hour))
	require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(nextDay, msg))
}

// TestAdmissionAuthorizeAnyBoundLicenseType: authorizing an activation key
// names no node type — no node exists yet — so its gate is "holds a license of
// any license type backing a registered node type". Licenses of a type nothing
// is bound to do not count.
func TestAdmissionAuthorizeAnyBoundLicenseType(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()

	authorize := func() error {
		return f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgAuthorizeActivationKey{
			Operator: operator, ActivationAddress: sample.AccAddress(),
		})
	}

	// Licences of a type no node type is bound to buy nothing.
	f.IssueLicensesOfType(t, operator, "nodelabs.node.unbound", 5)
	require.ErrorIs(t, authorize(), types.ErrNoActiveLicenses)

	// A licence of any bound type admits. Nano is enough even though the
	// operator holds no trust licences: authorizing a key names no node type,
	// so the gate cannot be narrower than "some bound type".
	f.IssueLicensesOfType(t, operator, f.NanoLicenseType, 1)
	require.NoError(t, authorize())

	// And the msg path agrees with the ante path.
	_ = authorizeKey(t, f, ms, f.Ctx, operator)
}

// TestAdmissionActivate: standing requires an active key bound to the named
// licensed operator; attempts burn the per-key daily counter against the
// spam ceiling.
func TestAdmissionActivate(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	// Unknown key.
	err := f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: sample.AccAddress(), Operator: operator, NodeAddress: sample.AccAddress(), NodeType: f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// Key bound to another operator.
	err = f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgActivateNode{
		ActivationAddress: key, Operator: sample.AccAddress(), NodeAddress: sample.AccAddress(), NodeType: f.NodeType,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	msg := &types.MsgActivateNode{ActivationAddress: key, Operator: operator, NodeAddress: sample.AccAddress(), NodeType: f.NodeType}

	params, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)
	spamLimit := params.SpamLimitMultiplier // 1 license

	for i := uint64(0); i < spamLimit; i++ {
		require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg))
	}
	require.ErrorIs(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg), types.ErrQuotaExceeded)

	// Disabled keys lose standing immediately.
	_, err = ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{Operator: operator, ActivationAddress: key})
	require.NoError(t, err)
	require.ErrorIs(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg), types.ErrUnauthorized)
}

// TestAdmissionActivateQuotaIsPerNodeType: the activate counter is scoped to
// the node type because the ceiling it is measured against is. An operator
// well licensed for one node type and sparsely licensed for another gets two
// independent daily allowances, so working through the generous one cannot
// exhaust the other before a single node of it is activated.
func TestAdmissionActivateQuotaIsPerNodeType(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()

	params, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)

	// 5 trust licences (ceiling 5x) against 1 nano licence (ceiling 1x).
	f.IssueLicensesOfType(t, operator, f.LicenseType, 5)
	f.IssueLicensesOfType(t, operator, f.NanoLicenseType, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	activate := func(nodeType string) error {
		return f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgActivateNode{
			ActivationAddress: key, Operator: operator,
			NodeAddress: sample.AccAddress(), NodeType: nodeType,
		})
	}

	// Spend the whole nano allowance...
	nanoLimit := params.SpamLimitMultiplier // 1 nano licence
	for i := uint64(0); i < nanoLimit; i++ {
		require.NoError(t, activate(f.NanoNodeType), "nano attempt %d", i)
	}
	require.ErrorIs(t, activate(f.NanoNodeType), types.ErrQuotaExceeded)

	// ...and the trust allowance is untouched by it: a pooled counter would
	// already be nanoLimit deep here.
	trustLimit := params.SpamLimitMultiplier * 5
	require.Greater(t, trustLimit, nanoLimit)
	for i := uint64(0); i < trustLimit; i++ {
		require.NoError(t, activate(f.NodeType), "trust attempt %d", i)
	}
	require.ErrorIs(t, activate(f.NodeType), types.ErrQuotaExceeded)

	// Both counters roll independently on the next UTC day.
	nextDay := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(24 * time.Hour))
	require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(nextDay, &types.MsgActivateNode{
		ActivationAddress: key, Operator: operator,
		NodeAddress: sample.AccAddress(), NodeType: f.NanoNodeType,
	}))
}

// TestAdmissionDeactivate mirrors the handler's signer rule and is not
// license-gated: winding down a fleet stays possible after revocation.
func TestAdmissionDeactivate(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	// Revoke the operator's licenses: deactivation must still be admitted.
	f.RevokeLicenses(t, operator, 1)

	// Unrelated signer.
	err := f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgDeactivateNode{Signer: sample.AccAddress(), NodeAddress: node})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	// Operator, node, and the original active key all have standing.
	for _, signer := range []string{operator, node, key} {
		require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgDeactivateNode{Signer: signer, NodeAddress: node}))
	}

	// A deauthorized key loses standing.
	_, err = ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{Operator: operator, ActivationAddress: key})
	require.NoError(t, err)
	err = f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgDeactivateNode{Signer: key, NodeAddress: node})
	require.ErrorIs(t, err, types.ErrKeyDisabled)

	// Deactivated nodes are rejected at admission.
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
	require.NoError(t, err)
	err = f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
	require.ErrorIs(t, err, types.ErrNodeDeactivated)
}

// TestAdmissionNodeStatus: quota burns per node per day and the license gate
// fires on the first counted tx of each UTC day.
func TestAdmissionNodeStatus(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)
	msg := &types.MsgUpdateNodeStatus{NodeAddress: node}

	params, err := f.Keeper.GetParams(f.Ctx)
	require.NoError(t, err)

	for i := uint64(0); i < params.StatusDailyLimit; i++ {
		require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg))
	}
	require.ErrorIs(t, f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, msg), types.ErrQuotaExceeded)

	// The stored counter sits exactly at the limit (the rejected attempt's
	// increment is never persisted), so the handler's re-read — which only
	// rejects counters already beyond the limit — still admits the tx the
	// ante admitted. The handler-side rejection of an over-limit counter is
	// covered by TestUpdateNodeStatusQuotaReread.
	counter, err := f.Keeper.NodeStatusCounters.Get(f.Ctx, node)
	require.NoError(t, err)
	require.Equal(t, params.StatusDailyLimit, counter.DailyCount)

	// License revoked: same-day txs still pass (the gate checks on roll)...
	f.RevokeLicenses(t, operator, 1)
	nextDay := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(24 * time.Hour))

	// ...but the first tx of the next day is rejected: a revoked operator's
	// fleet does not keep its free tx channel.
	require.ErrorIs(t, f.Keeper.CheckAndConsumeGaslessQuota(nextDay, msg), types.ErrNoActiveLicenses)

	// Unknown node fails closed.
	err = f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgUpdateNodeStatus{NodeAddress: sample.AccAddress()})
	require.ErrorIs(t, err, types.ErrNodeNotFound)
}

// TestAdmissionUnroutedMsg: msgs outside the module's gasless set fail
// closed.
func TestAdmissionUnroutedMsg(t *testing.T) {
	f, _ := setup(t)
	err := f.Keeper.CheckAndConsumeGaslessQuota(f.Ctx, &types.MsgDeauthorizeActivationKey{})
	require.ErrorIs(t, err, types.ErrUnauthorized)
}
