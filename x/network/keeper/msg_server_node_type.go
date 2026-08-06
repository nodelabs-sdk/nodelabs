package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// CreateNodeType registers a node type and binds it to a license type.
//
// Registration is gated on the module-wide "nodetype.create" grant, which is
// the whole authorization: holding it authorizes defining node types against
// any existing license type. The signing address is not recorded on the
// record, because it would confer nothing after the fact.
//
// The license type must exist. Binding to an absent one would mint a node type
// that can never activate anything — the activation limit counts licenses of
// that type, so the count would be permanently zero — and node types cannot be
// removed or re-pointed to undo it.
//
// The binding is one-to-one: a node type names exactly one license type, and a
// license type backs at most one node type. It is also fixed at creation, and
// node type records are never removed — activation resolves a node's type
// through this registry, so a type that vanished or re-pointed would strand
// every node already carrying it.
func (ms msgServer) CreateNodeType(ctx context.Context, msg *types.MsgCreateNodeType) (*types.MsgCreateNodeTypeResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	allowed, err := ms.k.can(ctx, msg.Creator, types.ActionNodeTypeCreate)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not hold the %s grant", msg.Creator, types.ActionNodeTypeCreate)
	}

	// Node type ids are single-use: the existing record is what reserves the
	// id, so a duplicate is rejected rather than overwriting a live binding.
	exists, err := ms.k.NodeTypes.Has(ctx, msg.Id)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, errorsmod.Wrapf(types.ErrNodeTypeExists, "node type %s", msg.Id)
	}

	// The binding is one-to-one: a license type backs at most one node type.
	// Checked before the existence lookup so the caller learns the license type
	// is taken rather than re-reading a type they cannot use anyway.
	bound, err := ms.k.NodeTypeByLicenseType.Get(ctx, msg.LicenseTypeId)
	switch {
	case err == nil:
		return nil, errorsmod.Wrapf(types.ErrLicenseTypeBound, "license type %s already backs node type %s", msg.LicenseTypeId, bound)
	case !errors.Is(err, collections.ErrNotFound):
		return nil, err
	}

	found, err := ms.k.licenseKeeper.HasLicenseType(ctx, msg.LicenseTypeId)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, errorsmod.Wrapf(types.ErrLicenseTypeNotFound, "license type %s", msg.LicenseTypeId)
	}

	nodeType := types.NodeType{
		Id:            msg.Id,
		LicenseTypeId: msg.LicenseTypeId,
	}
	if err := ms.k.NodeTypes.Set(ctx, msg.Id, nodeType); err != nil {
		return nil, err
	}
	// Written once and never moved: the binding is immutable, so this map
	// cannot drift from the record it mirrors.
	if err := ms.k.NodeTypeByLicenseType.Set(ctx, msg.LicenseTypeId, msg.Id); err != nil {
		return nil, err
	}

	// The signer is emitted as the event's signer rather than stored: indexers
	// can still attribute the registration, without state implying authority.
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeCreateNodeType,
		sdk.NewAttribute(types.AttributeKeyNodeType, msg.Id),
		sdk.NewAttribute(types.AttributeKeySigner, msg.Creator),
		sdk.NewAttribute(types.AttributeKeyLicenseTypeID, msg.LicenseTypeId),
	))

	return &types.MsgCreateNodeTypeResponse{}, nil
}
