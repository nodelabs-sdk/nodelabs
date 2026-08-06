package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

type msgServer struct {
	k Keeper
}

var _ types.MsgServer = msgServer{}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{k: keeper}
}

// CreateOperatorAccount creates an on-chain account for an operator wallet.
// The admin fallback for wallets that predate account creation on license
// issuance or hold no licenses yet.
func (ms msgServer) CreateOperatorAccount(ctx context.Context, msg *types.MsgCreateOperatorAccount) (*types.MsgCreateOperatorAccountResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	allowed, err := ms.k.can(ctx, msg.Admin, types.ActionWalletCreate)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not hold the %s grant", msg.Admin, types.ActionWalletCreate)
	}

	wallet, err := sdk.AccAddressFromBech32(msg.Wallet)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid wallet address %q: %s", msg.Wallet, err)
	}
	if ms.k.accountKeeper.HasAccount(ctx, wallet) {
		return nil, errorsmod.Wrapf(types.ErrAccountExists, "wallet %s", msg.Wallet)
	}

	ms.k.CreateAccountIfNotExists(ctx, wallet)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeCreateOperatorAccount,
		sdk.NewAttribute(types.AttributeKeyWallet, msg.Wallet),
		sdk.NewAttribute(types.AttributeKeySigner, msg.Admin),
	))

	return &types.MsgCreateOperatorAccountResponse{}, nil
}

// AuthorizeActivationKey authorizes a new activation key for the signing
// operator. Activation addresses are globally unique and permanently bound
// to the first operator that authorizes them; disabled records also block
// re-authorization (the address is burned).
func (ms msgServer) AuthorizeActivationKey(ctx context.Context, msg *types.MsgAuthorizeActivationKey) (*types.MsgAuthorizeActivationKeyResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if msg.ActivationAddress == msg.Operator {
		return nil, types.ErrSelfAuthorization
	}

	has, err := ms.k.ActivationKeys.Has(ctx, msg.ActivationAddress)
	if err != nil {
		return nil, err
	}
	if has {
		return nil, errorsmod.Wrapf(types.ErrKeyExists, "activation address %s", msg.ActivationAddress)
	}

	params, err := ms.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}

	active, recent, err := ms.k.countOperatorKeys(ctx, msg.Operator, params.RecentKeyWindow, sdkCtx.BlockTime())
	if err != nil {
		return nil, err
	}
	if active >= params.MaxActivationKeys {
		return nil, errorsmod.Wrapf(types.ErrTooManyActivationKeys, "operator %s has %d active keys (limit %d)", msg.Operator, active, params.MaxActivationKeys)
	}
	if recent >= params.RecentKeyLimit {
		return nil, errorsmod.Wrapf(types.ErrRecentKeyLimit, "operator %s created %d keys in the last %s (limit %d)", msg.Operator, recent, params.RecentKeyWindow, params.RecentKeyLimit)
	}

	key := types.ActivationKey{
		Address:   msg.ActivationAddress,
		Operator:  msg.Operator,
		CreatedAt: sdkCtx.BlockTime(),
		Status:    types.KeyActive,
	}
	if err := ms.k.ActivationKeys.Set(ctx, msg.ActivationAddress, key); err != nil {
		return nil, err
	}
	if err := ms.k.OperatorActivationKeys.Set(ctx, collections.Join(msg.Operator, msg.ActivationAddress)); err != nil {
		return nil, err
	}
	if err := ms.k.registerOperator(ctx, msg.Operator); err != nil {
		return nil, err
	}

	activationAddr, err := sdk.AccAddressFromBech32(msg.ActivationAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid activation address %q: %s", msg.ActivationAddress, err)
	}
	ms.k.CreateAccountIfNotExists(ctx, activationAddr)

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeAuthorizeActivationKey,
		sdk.NewAttribute(types.AttributeKeyOperator, msg.Operator),
		sdk.NewAttribute(types.AttributeKeyActivationAddress, msg.ActivationAddress),
	))

	return &types.MsgAuthorizeActivationKeyResponse{}, nil
}

// DeauthorizeActivationKey disables an activation key. The record is
// retained for the recent-creation count and to keep the address burned. On
// top of regular gas, the handler charges the deauthorize_fee param as a
// bank transfer to the fee collector — a fee-checker-immune cost (an ante
// check against the declared fee is bypassable under dynamic-fee semantics).
func (ms msgServer) DeauthorizeActivationKey(ctx context.Context, msg *types.MsgDeauthorizeActivationKey) (*types.MsgDeauthorizeActivationKeyResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	key, err := ms.k.ActivationKeys.Get(ctx, msg.ActivationAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrKeyNotFound, "activation address %s", msg.ActivationAddress)
	}
	if key.Operator != msg.Operator {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "activation key %s is not bound to operator %s", msg.ActivationAddress, msg.Operator)
	}
	if key.Status != types.KeyActive {
		return nil, errorsmod.Wrapf(types.ErrKeyDisabled, "activation key %s is already disabled", msg.ActivationAddress)
	}

	params, err := ms.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	if !params.DeauthorizeFee.IsZero() {
		operator, err := sdk.AccAddressFromBech32(msg.Operator)
		if err != nil {
			return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid operator address %q: %s", msg.Operator, err)
		}
		if err := ms.k.bankKeeper.SendCoinsFromAccountToModule(ctx, operator, authtypes.FeeCollectorName, params.DeauthorizeFee); err != nil {
			return nil, errorsmod.Wrapf(err, "charging deauthorize fee %s", params.DeauthorizeFee)
		}
	}

	key.Status = types.KeyDisabled
	if err := ms.k.ActivationKeys.Set(ctx, msg.ActivationAddress, key); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeDeauthorizeActivationKey,
		sdk.NewAttribute(types.AttributeKeyOperator, msg.Operator),
		sdk.NewAttribute(types.AttributeKeyActivationAddress, msg.ActivationAddress),
		sdk.NewAttribute(types.AttributeKeyFee, params.DeauthorizeFee.String()),
	))

	return &types.MsgDeauthorizeActivationKeyResponse{}, nil
}

// DeactivateNode deactivates a node. Valid signers are the operator, the
// node's original activation key — only while that key is still active, so
// deauthorizing a provider fully revokes its power over already-activated
// nodes — or the node itself. Deactivation is irreversible and deliberately
// does not touch last_active_time or the recent-activity index: deactivated
// nodes keep counting in the recent window.
func (ms msgServer) DeactivateNode(ctx context.Context, msg *types.MsgDeactivateNode) (*types.MsgDeactivateNodeResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	node, err := ms.k.Nodes.Get(ctx, msg.NodeAddress)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrNodeNotFound, "node %s", msg.NodeAddress)
	}
	if node.Status != types.NodeActive {
		return nil, errorsmod.Wrapf(types.ErrNodeDeactivated, "node %s is already deactivated", msg.NodeAddress)
	}

	switch msg.Signer {
	case node.Operator, node.Address:
		// authorized
	case node.ActivatedBy:
		key, err := ms.k.ActivationKeys.Get(ctx, msg.Signer)
		if err != nil {
			return nil, errorsmod.Wrapf(types.ErrKeyNotFound, "activation key %s", msg.Signer)
		}
		if key.Status != types.KeyActive {
			return nil, errorsmod.Wrapf(types.ErrKeyDisabled, "activation key %s has been deauthorized and cannot deactivate nodes", msg.Signer)
		}
	default:
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s is not the node's operator, original activation key, or the node itself", msg.Signer)
	}

	node.Status = types.NodeDeactivated
	if err := ms.k.Nodes.Set(ctx, msg.NodeAddress, node); err != nil {
		return nil, err
	}

	// Keyed by the node's own type, so the decrement lands in the same tally
	// the activation incremented.
	counts, err := ms.k.GetOperatorNodeCounts(ctx, node.Operator, node.Type)
	if err != nil {
		return nil, err
	}
	if counts.Active > 0 {
		counts.Active--
	}
	if err := ms.k.OperatorNodeCounts.Set(ctx, collections.Join(node.Operator, node.Type), counts); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeDeactivateNode,
		sdk.NewAttribute(types.AttributeKeyOperator, node.Operator),
		sdk.NewAttribute(types.AttributeKeyNodeAddress, msg.NodeAddress),
		sdk.NewAttribute(types.AttributeKeySigner, msg.Signer),
	))

	return &types.MsgDeactivateNodeResponse{}, nil
}

// UpdateNodeStatus records a node's status heartbeat. The daily quota is
// consumed in the ante handler (so failing txs still burn it and over-limit
// spam is rejected at mempool admission); the handler re-reads the counter
// and never increments it. The payload is emitted as an event, not stored.
func (ms msgServer) UpdateNodeStatus(ctx context.Context, msg *types.MsgUpdateNodeStatus) (*types.MsgUpdateNodeStatusResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	node, active, err := ms.k.IsActiveNode(ctx, msg.NodeAddress)
	if err != nil {
		return nil, err
	}
	if node.Address == "" {
		return nil, errorsmod.Wrapf(types.ErrNodeNotFound, "node %s", msg.NodeAddress)
	}
	if !active {
		return nil, errorsmod.Wrapf(types.ErrNodeDeactivated, "node %s", msg.NodeAddress)
	}

	params, err := ms.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	counter, err := ms.k.NodeStatusCounters.Get(ctx, msg.NodeAddress)
	if err == nil && types.SameDay(counter.LatestTime, sdkCtx.BlockTime()) && counter.DailyCount > params.StatusDailyLimit {
		return nil, errorsmod.Wrapf(types.ErrQuotaExceeded, "node %s exceeded %d status updates today", msg.NodeAddress, params.StatusDailyLimit)
	}

	if err := ms.k.TouchNodeActivity(ctx, msg.NodeAddress); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeNodeStatus,
		sdk.NewAttribute(types.AttributeKeyNodeAddress, msg.NodeAddress),
		sdk.NewAttribute(types.AttributeKeyOperator, node.Operator),
		sdk.NewAttribute(types.AttributeKeyNodeType, node.Type),
		sdk.NewAttribute(types.AttributeKeyDevice, msg.Payload.Device),
		sdk.NewAttribute(types.AttributeKeyOS, msg.Payload.Os),
		sdk.NewAttribute(types.AttributeKeyHostname, msg.Payload.Hostname),
		sdk.NewAttribute(types.AttributeKeyNodeInfo, msg.Payload.NodeInfo),
		sdk.NewAttribute(types.AttributeKeyWorkloads, strings.Join(msg.Payload.Workloads, ",")),
		sdk.NewAttribute(types.AttributeKeyMemory, msg.Payload.Memory),
		sdk.NewAttribute(types.AttributeKeyCPU, msg.Payload.Cpu),
		sdk.NewAttribute(types.AttributeKeyStorage, msg.Payload.Storage),
	))

	return &types.MsgUpdateNodeStatusResponse{}, nil
}

// UpdateParams updates the module parameters, including the module owner.
// Signer must be the module authority (x/gov by default). It is how an owner
// is first established outside genesis, and the recovery path when an owner
// key is lost or compromised — so it deliberately does not require the current
// owner's cooperation.
func (ms msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.k.authority != msg.Authority {
		return nil, errorsmod.Wrapf(govtypes.ErrInvalidSigner, "invalid authority; expected %s, got %s", ms.k.authority, msg.Authority)
	}
	if err := msg.Params.Validate(); err != nil {
		return nil, err
	}
	if err := ms.k.Params.Set(ctx, msg.Params); err != nil {
		return nil, err
	}
	return &types.MsgUpdateParamsResponse{}, nil
}

// ---------------------------------------------------------------------------
// Ownership and access grants
// ---------------------------------------------------------------------------

// TransferOwnership hands the module to a new owner. The current owner signs.
//
// This is the one path by which a non-authority signer writes a parameter, so
// it reads the current set back and replaces only Owner. The rest of this
// module's parameter set governs activation limits and gasless admission; a
// handoff that reset any of them to zero would silently change chain policy.
func (ms msgServer) TransferOwnership(ctx context.Context, msg *types.MsgTransferOwnership) (*types.MsgTransferOwnershipResponse, error) {
	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	if err := types.ValidateCanonicalAddress("new owner", msg.NewOwner); err != nil {
		return nil, err
	}

	params, err := ms.k.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	params.Owner = msg.NewOwner
	if err := ms.k.Params.Set(ctx, params); err != nil {
		return nil, err
	}

	sdk.UnwrapSDKContext(ctx).EventManager().EmitEvent(
		accesstypes.TransferOwnershipEvent(types.ModuleName, msg.NewOwner))

	return &types.MsgTransferOwnershipResponse{}, nil
}

// GrantAccess merges the incoming grants into the grantee's existing ones.
// Nothing is ever removed by this message; use RevokeAccess for that.
//
// Every entry must carry an empty scopes list. The shared grant store treats
// scopes as unconstrained opaque strings when a module declares no scope
// validator, which this module does not — so a scoped grant would be stored
// happily and then never found, because every check here looks under the empty
// scope. Rejecting it up front turns a silently dead grant into an error.
func (ms msgServer) GrantAccess(ctx context.Context, msg *types.MsgGrantAccess) (*types.MsgGrantAccessResponse, error) {
	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	if err := types.ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return nil, err
	}

	for i, entry := range msg.Grants {
		if len(entry.Scopes) > 0 {
			return nil, errorsmod.Wrapf(accesstypes.ErrInvalidScope,
				"grant %d: action %q is module-wide and takes no scopes, got %v", i, entry.Action, entry.Scopes)
		}
	}

	if err := ms.k.Grants.GrantTo(ctx, msg.Grantee, msg.Grants); err != nil {
		return nil, err
	}

	return &types.MsgGrantAccessResponse{}, nil
}

// RevokeAccess removes specific actions from a grantee. Actions that are not
// currently granted are silently ignored, so the same revoke can be safely
// re-sent.
//
// A non-empty scope is rejected rather than ignored: grants here only ever
// exist under the empty scope, so a scoped revoke could not remove anything
// and the caller would be told nothing.
func (ms msgServer) RevokeAccess(ctx context.Context, msg *types.MsgRevokeAccess) (*types.MsgRevokeAccessResponse, error) {
	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	// Validated here as well as in ValidateBasic: a non-canonical grantee is
	// not merely malformed, it names a key this revoke could never match, and
	// RevokeFrom reports a miss as success.
	if err := types.ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return nil, err
	}

	for i, pair := range msg.Actions {
		if pair.Scope != "" {
			return nil, errorsmod.Wrapf(accesstypes.ErrInvalidScope,
				"action %d: %q is module-wide and takes no scope, got %q", i, pair.Action, pair.Scope)
		}
	}

	if err := ms.k.Grants.RevokeFrom(ctx, msg.Grantee, msg.Actions); err != nil {
		return nil, err
	}

	return &types.MsgRevokeAccessResponse{}, nil
}
