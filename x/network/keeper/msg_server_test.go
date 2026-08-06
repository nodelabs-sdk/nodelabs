package keeper_test

import (
	"errors"
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

func setup(t testing.TB) (*keepertest.NetworkFixture, types.MsgServer) {
	t.Helper()
	f := keepertest.NewNetworkFixture(t)
	return f, keeper.NewMsgServerImpl(f.Keeper)
}

// authorizeKey authorizes a fresh activation key for operator and returns
// its address.
func authorizeKey(t testing.TB, f *keepertest.NetworkFixture, ms types.MsgServer, ctx sdk.Context, operator string) string {
	t.Helper()
	addr := sample.AccAddress()
	_, err := ms.AuthorizeActivationKey(ctx, &types.MsgAuthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: addr,
	})
	require.NoError(t, err)
	return addr
}

// activateNode activates a fresh node via key for operator and returns its
// address.
func activateNode(t testing.TB, f *keepertest.NetworkFixture, ms types.MsgServer, ctx sdk.Context, key, operator string) string {
	t.Helper()
	addr := sample.AccAddress()
	_, err := ms.ActivateNode(ctx, &types.MsgActivateNode{
		ActivationAddress: key,
		Operator:          operator,
		NodeAddress:       addr,
		NodeType:          f.NodeType,
	})
	require.NoError(t, err)
	return addr
}

// setParams mutates the fixture params through fn.
func setParams(t testing.TB, f *keepertest.NetworkFixture, fn func(*types.Params)) {
	t.Helper()
	params, err := f.Keeper.Params.Get(f.Ctx)
	require.NoError(t, err)
	fn(&params)
	require.NoError(t, f.Keeper.Params.Set(f.Ctx, params))
}

// ---------------------------------------------------------------------------
// AuthorizeActivationKey
// ---------------------------------------------------------------------------

func TestAuthorizeActivationKey(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)

	// Self-authorization is rejected.
	_, err := ms.AuthorizeActivationKey(f.Ctx, &types.MsgAuthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: operator,
	})
	require.ErrorIs(t, err, types.ErrSelfAuthorization)

	key := authorizeKey(t, f, ms, f.Ctx, operator)

	// The record is written, the operator registered, and the activation
	// account created.
	rec, err := f.Keeper.ActivationKeys.Get(f.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, operator, rec.Operator)
	require.Equal(t, types.KeyActive, rec.Status)
	require.Equal(t, keepertest.NetworkFixtureBlockTime.Unix(), rec.CreatedAt.Unix())

	registered, err := f.Keeper.Operators.Has(f.Ctx, operator)
	require.NoError(t, err)
	require.True(t, registered)

	keyAddr, err := sdk.AccAddressFromBech32(key)
	require.NoError(t, err)
	require.True(t, f.AccountKeeper.HasAccount(f.Ctx, keyAddr))

	// Global uniqueness: the same address cannot be authorized again, by the
	// same operator or any other.
	for _, op := range []string{operator, sample.AccAddress()} {
		_, err = ms.AuthorizeActivationKey(f.Ctx, &types.MsgAuthorizeActivationKey{
			Operator:          op,
			ActivationAddress: key,
		})
		require.ErrorIs(t, err, types.ErrKeyExists)
	}
}

func TestAuthorizeActivationKeyBurnedAfterDeauthorize(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)

	key := authorizeKey(t, f, ms, f.Ctx, operator)
	_, err := ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: key,
	})
	require.NoError(t, err)

	// A deauthorized address is burned: no operator can ever re-authorize
	// it, including the original one.
	for _, op := range []string{sample.AccAddress(), operator} {
		_, err = ms.AuthorizeActivationKey(f.Ctx, &types.MsgAuthorizeActivationKey{
			Operator:          op,
			ActivationAddress: key,
		})
		require.ErrorIs(t, err, types.ErrKeyExists)
	}
}

func TestAuthorizeActivationKeyLimits(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	setParams(t, f, func(p *types.Params) {
		p.MaxActivationKeys = 2
		p.RecentKeyLimit = 3
	})

	k1 := authorizeKey(t, f, ms, f.Ctx, operator)
	k2 := authorizeKey(t, f, ms, f.Ctx, operator)

	// Third active key exceeds max_activation_keys.
	_, err := ms.AuthorizeActivationKey(f.Ctx, &types.MsgAuthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrTooManyActivationKeys)

	// Disabling one frees an active slot; the disabled key still counts
	// toward the recent-creation window.
	_, err = ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: k1,
	})
	require.NoError(t, err)
	authorizeKey(t, f, ms, f.Ctx, operator)

	// With another active slot freed, the fourth creation within the window
	// still exceeds recent_key_limit: disabled keys keep counting there.
	_, err = ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: k2,
	})
	require.NoError(t, err)
	_, err = ms.AuthorizeActivationKey(f.Ctx, &types.MsgAuthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrRecentKeyLimit)

	// Once the window passes, creation is limited by active keys again.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(25 * time.Hour))
	authorizeKey(t, f, ms, later, operator)
	_, err = ms.AuthorizeActivationKey(later, &types.MsgAuthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrTooManyActivationKeys)
}

// ---------------------------------------------------------------------------
// DeauthorizeActivationKey
// ---------------------------------------------------------------------------

func TestDeauthorizeActivationKey(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	tests := []struct {
		name   string
		input  *types.MsgDeauthorizeActivationKey
		expErr error
	}{
		{
			name: "unknown key",
			input: &types.MsgDeauthorizeActivationKey{
				Operator:          operator,
				ActivationAddress: sample.AccAddress(),
			},
			expErr: types.ErrKeyNotFound,
		},
		{
			name: "wrong operator",
			input: &types.MsgDeauthorizeActivationKey{
				Operator:          sample.AccAddress(),
				ActivationAddress: key,
			},
			expErr: types.ErrUnauthorized,
		},
		{
			name: "valid",
			input: &types.MsgDeauthorizeActivationKey{
				Operator:          operator,
				ActivationAddress: key,
			},
		},
		{
			name: "already disabled",
			input: &types.MsgDeauthorizeActivationKey{
				Operator:          operator,
				ActivationAddress: key,
			},
			expErr: types.ErrKeyDisabled,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.DeauthorizeActivationKey(f.Ctx, tc.input)
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				return
			}
			require.NoError(t, err)
			rec, err := f.Keeper.ActivationKeys.Get(f.Ctx, key)
			require.NoError(t, err)
			require.Equal(t, types.KeyDisabled, rec.Status)
		})
	}

	// With an empty deauthorize_fee, no transfer is made.
	require.Empty(t, f.BankKeeper.Transfers)
}

func TestDeauthorizeActivationKeyFee(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	fee := sdk.NewCoins(sdk.NewInt64Coin("stake", 25))
	setParams(t, f, func(p *types.Params) { p.DeauthorizeFee = fee })

	key := authorizeKey(t, f, ms, f.Ctx, operator)

	// A transfer failure (operator cannot pay) rejects the deauthorization
	// and leaves the key active.
	f.BankKeeper.FailWith = errors.New("insufficient funds")
	_, err := ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: key,
	})
	require.ErrorContains(t, err, "insufficient funds")
	rec, err := f.Keeper.ActivationKeys.Get(f.Ctx, key)
	require.NoError(t, err)
	require.Equal(t, types.KeyActive, rec.Status)

	// A successful deauthorization transfers the fee to the fee collector.
	f.BankKeeper.FailWith = nil
	_, err = ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: key,
	})
	require.NoError(t, err)
	require.Len(t, f.BankKeeper.Transfers, 1)
	require.Equal(t, operator, f.BankKeeper.Transfers[0].From)
	require.Equal(t, "fee_collector", f.BankKeeper.Transfers[0].Module)
	require.Equal(t, fee, f.BankKeeper.Transfers[0].Amount)
}

// ---------------------------------------------------------------------------
// DeactivateNode
// ---------------------------------------------------------------------------

func TestDeactivateNodeSigners(t *testing.T) {
	tests := []struct {
		name   string
		signer func(operator, key, node string) string
		expErr error
	}{
		{name: "operator", signer: func(op, _, _ string) string { return op }},
		{name: "original activation key", signer: func(_, key, _ string) string { return key }},
		{name: "the node itself", signer: func(_, _, node string) string { return node }},
		{name: "unrelated signer", signer: func(_, _, _ string) string { return sample.AccAddress() }, expErr: types.ErrUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f, ms := setup(t)
			operator := sample.AccAddress()
			f.IssueLicenses(t, operator, 1)
			key := authorizeKey(t, f, ms, f.Ctx, operator)
			node := activateNode(t, f, ms, f.Ctx, key, operator)

			_, err := ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{
				Signer:      tc.signer(operator, key, node),
				NodeAddress: node,
			})
			if tc.expErr != nil {
				require.ErrorIs(t, err, tc.expErr)
				return
			}
			require.NoError(t, err)

			rec, err := f.Keeper.Nodes.Get(f.Ctx, node)
			require.NoError(t, err)
			require.Equal(t, types.NodeDeactivated, rec.Status)

			counts, err := f.Keeper.GetOperatorNodeCounts(f.Ctx, operator, f.NodeType)
			require.NoError(t, err)
			require.Equal(t, uint64(1), counts.Total)
			require.Equal(t, uint64(0), counts.Active)

			// Deactivating again fails.
			_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{
				Signer:      operator,
				NodeAddress: node,
			})
			require.ErrorIs(t, err, types.ErrNodeDeactivated)
		})
	}
}

// TestDeactivateNodeDeauthorizedKey pins the deliberate refinement:
// deauthorizing a provider's key fully revokes its power over
// already-activated nodes.
func TestDeactivateNodeDeauthorizedKey(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	_, err := ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: key,
	})
	require.NoError(t, err)

	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{
		Signer:      key,
		NodeAddress: node,
	})
	require.ErrorIs(t, err, types.ErrKeyDisabled)

	// The operator still can.
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{
		Signer:      operator,
		NodeAddress: node,
	})
	require.NoError(t, err)
}

func TestDeactivateNodeUnknown(t *testing.T) {
	f, ms := setup(t)
	_, err := ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{
		Signer:      sample.AccAddress(),
		NodeAddress: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrNodeNotFound)
}

// ---------------------------------------------------------------------------
// UpdateNodeStatus
// ---------------------------------------------------------------------------

func TestUpdateNodeStatus(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	// Unknown node.
	_, err := ms.UpdateNodeStatus(f.Ctx, &types.MsgUpdateNodeStatus{
		NodeAddress: sample.AccAddress(),
	})
	require.ErrorIs(t, err, types.ErrNodeNotFound)

	// Valid update advances last_active_time and emits the payload event.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(2 * time.Hour))
	_, err = ms.UpdateNodeStatus(later, &types.MsgUpdateNodeStatus{
		NodeAddress: node,
		Payload: types.NodeStatusPayload{
			Os:       "linux",
			Hostname: "node-1",
		},
	})
	require.NoError(t, err)

	rec, err := f.Keeper.Nodes.Get(f.Ctx, node)
	require.NoError(t, err)
	require.Equal(t, later.BlockTime().Unix(), rec.LastActiveTime.Unix())

	found := false
	for _, ev := range later.EventManager().Events() {
		if ev.Type == types.EventTypeNodeStatus {
			found = true
		}
	}
	require.True(t, found, "node_status event not emitted")

	// A deactivated node cannot report status.
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
	require.NoError(t, err)
	_, err = ms.UpdateNodeStatus(f.Ctx, &types.MsgUpdateNodeStatus{NodeAddress: node})
	require.ErrorIs(t, err, types.ErrNodeDeactivated)
}

// TestUpdateNodeStatusQuotaReread pins the handler side of the quota
// contract: the ante increments the counter, the handler only re-reads it.
func TestUpdateNodeStatusQuotaReread(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	params, err := f.Keeper.Params.Get(f.Ctx)
	require.NoError(t, err)

	// An over-limit counter for today rejects the update.
	require.NoError(t, f.Keeper.NodeStatusCounters.Set(f.Ctx, node, types.ActivityCounter{
		LatestTime: f.Ctx.BlockTime(),
		DailyCount: params.StatusDailyLimit + 1,
	}))
	_, err = ms.UpdateNodeStatus(f.Ctx, &types.MsgUpdateNodeStatus{NodeAddress: node})
	require.ErrorIs(t, err, types.ErrQuotaExceeded)

	// A stale over-limit counter from a previous day does not.
	require.NoError(t, f.Keeper.NodeStatusCounters.Set(f.Ctx, node, types.ActivityCounter{
		LatestTime: f.Ctx.BlockTime().Add(-24 * time.Hour),
		DailyCount: params.StatusDailyLimit + 1,
	}))
	_, err = ms.UpdateNodeStatus(f.Ctx, &types.MsgUpdateNodeStatus{NodeAddress: node})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// CreateOperatorAccount
// ---------------------------------------------------------------------------

func TestCreateOperatorAccount(t *testing.T) {
	f, ms := setup(t)
	admin := sample.AccAddress()
	wallet := sample.AccAddress()

	// Without the wallet.create grant.
	_, err := ms.CreateOperatorAccount(f.Ctx, &types.MsgCreateOperatorAccount{
		Admin:  admin,
		Wallet: wallet,
	})
	require.ErrorIs(t, err, types.ErrUnauthorized)

	f.GrantNetwork(t, admin, types.ActionWalletCreate)

	_, err = ms.CreateOperatorAccount(f.Ctx, &types.MsgCreateOperatorAccount{
		Admin:  admin,
		Wallet: wallet,
	})
	require.NoError(t, err)

	walletAddr, err := sdk.AccAddressFromBech32(wallet)
	require.NoError(t, err)
	require.True(t, f.AccountKeeper.HasAccount(f.Ctx, walletAddr))

	// Creating an existing account is an error.
	_, err = ms.CreateOperatorAccount(f.Ctx, &types.MsgCreateOperatorAccount{
		Admin:  admin,
		Wallet: wallet,
	})
	require.ErrorIs(t, err, types.ErrAccountExists)
}

// ---------------------------------------------------------------------------
// UpdateParams
// ---------------------------------------------------------------------------

func TestUpdateParams(t *testing.T) {
	f, ms := setup(t)

	// Non-empty slices so the stored/loaded round-trip compares equal (proto
	// unmarshals empty repeated fields as nil).
	valid := types.DefaultParams()
	valid.DeauthorizeFee = sdk.NewCoins(sdk.NewInt64Coin("stake", 1))

	_, err := ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: sample.AccAddress(),
		Params:    valid,
	})
	require.ErrorContains(t, err, "invalid authority")

	invalid := types.DefaultParams()
	invalid.ActivationLimitMultiplier = 0
	_, err = ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: f.Keeper.GetAuthority(),
		Params:    invalid,
	})
	require.ErrorContains(t, err, "activation_limit_multiplier")

	_, err = ms.UpdateParams(f.Ctx, &types.MsgUpdateParams{
		Authority: f.Keeper.GetAuthority(),
		Params:    valid,
	})
	require.NoError(t, err)

	got, err := f.Keeper.Params.Get(f.Ctx)
	require.NoError(t, err)
	require.Equal(t, valid, got)
}
