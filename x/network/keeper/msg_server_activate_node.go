package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// ActivateNode activates a node under an operator, enforcing the
// activation-limit algorithm:
//
//  0. The signing activation key must exist, be active, and be bound to
//     msg.operator. Disabled keys cannot activate.
//  1. The node address must never have been activated (records are never
//     removed, so deactivated addresses stay burned).
//  2. count = active counted licenses; zero fails with a distinct "no active
//     licenses" error. limit = activation_limit_multiplier x count.
//  3. If the operator's all-time node total is below the limit, allow.
//  4. Else if the recent-active count (today + yesterday) is below the
//     limit, allow.
//  5. Else if the recent-active count has reached spam_limit_multiplier x
//     count, fail "recent activation limit exceeded".
//  6. Else the currently-active count decides against the limit.
func (ms msgServer) ActivateNode(ctx context.Context, msg *types.MsgActivateNode) (*types.MsgActivateNodeResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Step 0 — authorization. This is the core security property: only an
	// active key bound to the named operator may activate.
	key, err := ms.k.ActivationKeys.Get(ctx, msg.ActivationAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "activation key %s is not authorized", msg.ActivationAddress)
	}
	if key.Status != types.KeyActive {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "activation key %s is disabled", msg.ActivationAddress)
	}
	if key.Operator != msg.Operator {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "activation key %s is not bound to operator %s", msg.ActivationAddress, msg.Operator)
	}

	params, err := ms.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	// The node type registry is the allowlist, and it fail-closes: an empty
	// registry admits nothing. The record is fetched rather than probed with
	// Has because its license type is what the activation limit is built from.
	nodeType, err := ms.k.NodeTypes.Get(ctx, msg.NodeType)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidNodeType, "node type %q is not registered", msg.NodeType)
	}

	// Step 1 — node addresses are single-use.
	has, err := ms.k.Nodes.Has(ctx, msg.NodeAddress)
	if err != nil {
		return nil, err
	}
	if has {
		return nil, errorsmod.Wrapf(types.ErrNodeExists, "node %s", msg.NodeAddress)
	}

	// Gather the comparands first so the license walk can stop as soon as
	// the count is decisive: every check compares a multiplier x count
	// product against one of these, so counts beyond
	// max(comparands)/activation_limit_multiplier + 1 cannot change any
	// outcome (spam_limit_multiplier >= activation_limit_multiplier makes
	// the spam check decisive even earlier).
	//
	// Every quantity here is scoped to this node type — the tallies, the
	// activity window, and the license count alike. Mixing scopes would let an
	// operator's nodes of one type consume another type's allowance.
	counts, err := ms.k.GetOperatorNodeCounts(ctx, msg.Operator, msg.NodeType)
	if err != nil {
		return nil, err
	}
	recent, err := ms.k.CountRecentActiveNodes(ctx, msg.Operator, msg.NodeType, sdkCtx.BlockTime())
	if err != nil {
		return nil, err
	}

	maxComparand := counts.Total
	if counts.Active > maxComparand {
		maxComparand = counts.Active
	}
	if recent > maxComparand {
		maxComparand = recent
	}
	stopAt := maxComparand/params.ActivationLimitMultiplier + 1

	// Step 2 — license count and activation limit. Only licenses of the type
	// bound to this node type count; a node type is entitled by its own
	// license type and nothing else.
	count, err := ms.k.licenseKeeper.CountActiveLicenses(ctx, msg.Operator, []string{nodeType.LicenseTypeId}, stopAt)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		// Distinct error instead of the spec's literal fall-through, which
		// would surface a confusing "recent activation limit exceeded (0)".
		return nil, errorsmod.Wrapf(types.ErrNoActiveLicenses, "operator %s holds no active %s licenses", msg.Operator, nodeType.LicenseTypeId)
	}
	limit := params.ActivationLimitMultiplier * count

	// Steps 3-6.
	switch {
	case counts.Total < limit:
		// allow
	case recent < limit:
		// allow
	default:
		spamLimit := params.SpamLimitMultiplier * count
		if recent >= spamLimit {
			return nil, errorsmod.Wrapf(types.ErrRecentActivationLimit, "recent activation limit exceeded (%d)", spamLimit)
		}
		if counts.Active >= limit {
			return nil, errorsmod.Wrapf(types.ErrActivationLimit, "activation limit exceeded (%d)", limit)
		}
	}

	// Admitted: write the node, seed its recent-activity entry, and bump the
	// denormalized counts.
	node := types.Node{
		Address:        msg.NodeAddress,
		Operator:       msg.Operator,
		ActivatedBy:    msg.ActivationAddress,
		Type:           msg.NodeType,
		Status:         types.NodeActive,
		LastActiveTime: sdkCtx.BlockTime(),
	}
	if err := ms.k.Nodes.Set(ctx, msg.NodeAddress, node); err != nil {
		return nil, err
	}
	if err := ms.k.OperatorNodes.Set(ctx, collections.Join(msg.Operator, msg.NodeAddress)); err != nil {
		return nil, err
	}
	if err := ms.k.RecentNodeActivity.Set(ctx, collections.Join4(msg.Operator, msg.NodeType, types.DayEpoch(sdkCtx.BlockTime()), msg.NodeAddress)); err != nil {
		return nil, err
	}
	counts.Total++
	counts.Active++
	if err := ms.k.OperatorNodeCounts.Set(ctx, collections.Join(msg.Operator, msg.NodeType), counts); err != nil {
		return nil, err
	}
	if err := ms.k.registerOperator(ctx, msg.Operator); err != nil {
		return nil, err
	}

	nodeAddr, err := sdk.AccAddressFromBech32(msg.NodeAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid node address %q: %s", msg.NodeAddress, err)
	}
	ms.k.CreateAccountIfNotExists(ctx, nodeAddr)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeActivateNode,
		sdk.NewAttribute(types.AttributeKeyOperator, msg.Operator),
		sdk.NewAttribute(types.AttributeKeyActivationAddress, msg.ActivationAddress),
		sdk.NewAttribute(types.AttributeKeyNodeAddress, msg.NodeAddress),
		sdk.NewAttribute(types.AttributeKeyNodeType, msg.NodeType),
	))

	return &types.MsgActivateNodeResponse{}, nil
}
