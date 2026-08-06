package keeper

import (
	"context"
	"time"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// CheckAndConsumeGaslessQuota is the ante-side admission check for this
// module's gasless msgs: it verifies signer standing and rolls + increments
// the relevant daily counter. Because it runs in the ante chain, its writes
// are committed even when the msg handler later fails, and over-limit spam
// is rejected at CheckTx. Handlers re-read these counters and never
// re-increment.
//
// Per msg:
//   - MsgAuthorizeActivationKey: operator must hold >= 1 active counted
//     license (a licenseless address must not have a free tx channel);
//     counter per operator, limit recent_key_limit per day.
//   - MsgActivateNode: signing key must exist, be active, and be bound to
//     the named licensed operator; counter per activation key, limit
//     spam_limit_multiplier x license count per day (the same ceiling the
//     handler's recent-activity check enforces on successes).
//   - MsgDeactivateNode: signer must be the node's operator, its original
//     still-active activation key, or the node itself, and the node must be
//     active; counter per signer, limit status_daily_limit per day. Not
//     license-gated: winding down a fleet must stay possible after
//     revocation.
//   - MsgUpdateNodeStatus: node must be active; on the counter's first roll
//     of each UTC day the operator must still hold >= 1 active counted
//     license (a revoked operator's fleet must not keep its free tx channel
//     forever); counter per node, limit status_daily_limit per day.
func (k Keeper) CheckAndConsumeGaslessQuota(ctx context.Context, m sdk.Msg) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	blockTime := sdkCtx.BlockTime()
	height := sdkCtx.BlockHeight()

	params, err := k.GetParams(ctx)
	if err != nil {
		return err
	}

	switch msg := m.(type) {
	case *types.MsgAuthorizeActivationKey:
		if err := k.EnsureOperatorLicensed(ctx, msg.Operator); err != nil {
			return err
		}
		return k.consumeGaslessCounter(ctx, types.GaslessKindAuthorize, msg.Operator, params.RecentKeyLimit, blockTime, height)

	case *types.MsgActivateNode:
		key, found, err := k.GetActivationKey(ctx, msg.ActivationAddress)
		if err != nil {
			return err
		}
		if !found || key.Status != types.KeyActive || key.Operator != msg.Operator {
			return errorsmod.Wrapf(types.ErrUnauthorized, "activation key %s is not an active key of operator %s", msg.ActivationAddress, msg.Operator)
		}
		// The msg names its node type, so the gate is the narrow one.
		if err := k.EnsureOperatorLicensedForNodeType(ctx, msg.Operator, msg.NodeType); err != nil {
			return err
		}
		return k.consumeActivateQuota(ctx, msg, params, blockTime, height)

	case *types.MsgDeactivateNode:
		node, active, err := k.IsActiveNode(ctx, msg.NodeAddress)
		if err != nil {
			return err
		}
		if node.Address == "" {
			return errorsmod.Wrapf(types.ErrNodeNotFound, "node %s", msg.NodeAddress)
		}
		if !active {
			return errorsmod.Wrapf(types.ErrNodeDeactivated, "node %s", msg.NodeAddress)
		}
		if err := k.checkDeactivateSigner(ctx, node, msg.Signer); err != nil {
			return err
		}
		return k.consumeGaslessCounter(ctx, types.GaslessKindDeactivate, msg.Signer, params.StatusDailyLimit, blockTime, height)

	case *types.MsgUpdateNodeStatus:
		node, active, err := k.IsActiveNode(ctx, msg.NodeAddress)
		if err != nil {
			return err
		}
		if node.Address == "" {
			return errorsmod.Wrapf(types.ErrNodeNotFound, "node %s", msg.NodeAddress)
		}
		if !active {
			return errorsmod.Wrapf(types.ErrNodeDeactivated, "node %s", msg.NodeAddress)
		}

		counter, err := k.NodeStatusCounters.Get(ctx, msg.NodeAddress)
		if err != nil {
			counter = types.ActivityCounter{}
		}
		counter = rollCounter(counter, blockTime, height)
		// License gating of ongoing service, checked on the first counted tx
		// of each UTC day.
		if counter.DailyCount == 0 {
			// The node record carries its type, so this gate is the narrow
			// one too: an operator must still be licensed for *this* node's
			// type to keep it in service.
			if err := k.EnsureOperatorLicensedForNodeType(ctx, node.Operator, node.Type); err != nil {
				return err
			}
		}
		counter.DailyCount++
		if counter.DailyCount > params.StatusDailyLimit {
			return errorsmod.Wrapf(types.ErrQuotaExceeded, "node %s exceeded %d status updates today", msg.NodeAddress, params.StatusDailyLimit)
		}
		return k.NodeStatusCounters.Set(ctx, msg.NodeAddress, counter)

	default:
		return errorsmod.Wrapf(types.ErrUnauthorized, "%T is not a gasless msg of the network module", m)
	}
}

// consumeActivateQuota rolls the per-(key, node type) activation counter
// against the spam ceiling. The license walk stops as soon as the count is
// decisive for the attempted daily total.
//
// The counter is scoped to the node type because its ceiling is: the spam
// limit is derived from the operator's licenses of the one license type
// backing msg.NodeType. A counter pooled across node types would be measured
// against whichever type the current msg names, so a key working through a
// well-licensed type's generous allowance would exhaust the daily quota of a
// sparsely-licensed one before a single node of it was activated.
func (k Keeper) consumeActivateQuota(ctx context.Context, msg *types.MsgActivateNode, params types.Params, blockTime time.Time, height int64) error {
	key := collections.Join3(types.GaslessKindActivate, msg.ActivationAddress, msg.NodeType)
	counter, err := k.GaslessCounters.Get(ctx, key)
	if err != nil {
		counter = types.ActivityCounter{}
	}
	counter = rollCounter(counter, blockTime, height)
	counter.DailyCount++

	stopAt := counter.DailyCount/params.SpamLimitMultiplier + 1
	nodeType, err := k.NodeTypes.Get(ctx, msg.NodeType)
	if err != nil {
		return errorsmod.Wrapf(types.ErrInvalidNodeType, "node type %q is not registered", msg.NodeType)
	}
	count, err := k.licenseKeeper.CountActiveLicenses(ctx, msg.Operator, []string{nodeType.LicenseTypeId}, stopAt)
	if err != nil {
		return err
	}
	if counter.DailyCount > params.SpamLimitMultiplier*count {
		return errorsmod.Wrapf(types.ErrQuotaExceeded, "activation key %s exceeded %d %s activation attempts today", msg.ActivationAddress, params.SpamLimitMultiplier*count, msg.NodeType)
	}
	return k.GaslessCounters.Set(ctx, key, counter)
}

// checkDeactivateSigner mirrors the DeactivateNode handler's signer rule.
func (k Keeper) checkDeactivateSigner(ctx context.Context, node types.Node, signer string) error {
	switch signer {
	case node.Operator, node.Address:
		return nil
	case node.ActivatedBy:
		key, err := k.ActivationKeys.Get(ctx, signer)
		if err != nil {
			return errorsmod.Wrapf(types.ErrKeyNotFound, "activation key %s", signer)
		}
		if key.Status != types.KeyActive {
			return errorsmod.Wrapf(types.ErrKeyDisabled, "activation key %s has been deauthorized and cannot deactivate nodes", signer)
		}
		return nil
	default:
		return errorsmod.Wrapf(types.ErrUnauthorized, "%s is not the node's operator, original activation key, or the node itself", signer)
	}
}

// consumeGaslessCounter rolls and increments one daily counter against a flat
// limit. These kinds are measured against a param rather than a license-derived
// ceiling, so they have nothing to scope by and take the empty scope.
func (k Keeper) consumeGaslessCounter(ctx context.Context, kind, signer string, limit uint64, blockTime time.Time, height int64) error {
	key := collections.Join3(kind, signer, types.GaslessScopeNone)
	counter, err := k.GaslessCounters.Get(ctx, key)
	if err != nil {
		counter = types.ActivityCounter{}
	}
	counter = rollCounter(counter, blockTime, height)
	counter.DailyCount++
	if counter.DailyCount > limit {
		return errorsmod.Wrapf(types.ErrQuotaExceeded, "%s quota for %s exceeded (%d/day)", kind, signer, limit)
	}
	return k.GaslessCounters.Set(ctx, key, counter)
}
