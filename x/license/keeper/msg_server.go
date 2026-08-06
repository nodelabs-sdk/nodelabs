package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

type msgServer struct {
	k Keeper
}

var _ types.MsgServer = msgServer{}

func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{k: keeper}
}

// CreateLicenseType registers a new license type. The signer must hold the
// module-wide "type.create" grant. Module ownership alone does not authorize
// creation: as with "issue" and "revoke", the owner's role is to grant the
// right, including to itself, not to exercise it implicitly.
func (ms msgServer) CreateLicenseType(ctx context.Context, msg *types.MsgCreateLicenseType) (*types.MsgCreateLicenseTypeResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// The empty scope is the only key form a module-wide grant is stored
	// under; the grant store rejects any other at grant time.
	allowed, err := ms.k.can(ctx, msg.Creator, types.ActionCreateType, "")
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not hold the %s grant", msg.Creator, types.ActionCreateType)
	}

	if msg.Id == "" {
		return nil, errorsmod.Wrap(types.ErrLicenseTypeNotFound, "license type id cannot be empty")
	}

	if err := types.ValidateMaxSupply(msg.MaxSupply); err != nil {
		return nil, err
	}

	_, err = ms.k.LicenseTypes.Get(ctx, msg.Id)
	if err == nil {
		return nil, errorsmod.Wrapf(types.ErrLicenseTypeExists, "license type %s already exists", msg.Id)
	}

	// The signer is not recorded: the "type.create" grant is the whole
	// authorization, and downstream modules gate on their own grants rather
	// than on who created a license type.
	lt := types.LicenseType{
		Id:            msg.Id,
		Transferrable: msg.Transferrable,
		MaxSupply:     msg.MaxSupply,
		IssuedCount:   math.ZeroInt(),
		ActiveCount:   math.ZeroInt(),
		RevokedCount:  math.ZeroInt(),
	}

	if err := ms.k.LicenseTypes.Set(ctx, msg.Id, lt); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeCreateLicenseType,
		sdk.NewAttribute(types.AttributeKeyLicenseTypeID, msg.Id),
	))

	return &types.MsgCreateLicenseTypeResponse{}, nil
}

func (ms msgServer) UpdateLicenseType(ctx context.Context, msg *types.MsgUpdateLicenseType) (*types.MsgUpdateLicenseTypeResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	lt, err := ms.k.LicenseTypes.Get(ctx, msg.Id)
	if err != nil {
		return nil, errorsmod.Wrapf(types.ErrLicenseTypeNotFound, "license type %s not found", msg.Id)
	}

	// Only transferrability is updatable; max_supply is fixed at creation.
	lt.Transferrable = msg.Transferrable

	if err := ms.k.LicenseTypes.Set(ctx, msg.Id, lt); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeUpdateLicenseType,
		sdk.NewAttribute(types.AttributeKeyLicenseTypeID, msg.Id),
	))

	return &types.MsgUpdateLicenseTypeResponse{}, nil
}

// IssueLicenses issues licenses for each entry in the message. Each entry
// carries its own license type, holder, dates, and count; the signer must hold
// the "issue" grant for every referenced license type. All entries are
// validated before any license is issued, and the returned ids are flattened
// in entry order.
func (ms msgServer) IssueLicenses(ctx context.Context, msg *types.MsgIssueLicenses) (*types.MsgIssueLicensesResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if len(msg.Entries) == 0 {
		return nil, errorsmod.Wrap(types.ErrEmptyBatchEntries, "entries must not be empty")
	}
	if len(msg.Entries) > types.MaxIssueBatchSize {
		return nil, fmt.Errorf("entries length %d exceeds max batch size %d", len(msg.Entries), types.MaxIssueBatchSize)
	}

	for i, entry := range msg.Entries {
		if _, err := sdk.AccAddressFromBech32(entry.Holder); err != nil {
			return nil, fmt.Errorf("entry %d: invalid holder address %q: %w", i, entry.Holder, err)
		}
		if err := types.ValidateDates(entry.StartDate, entry.EndDate); err != nil {
			return nil, fmt.Errorf("entry %d: %w", i, err)
		}
		if entry.Count == 0 {
			return nil, errorsmod.Wrapf(types.ErrInvalidCount, "entry %d: count must be greater than zero", i)
		}

		allowed, err := ms.k.can(ctx, msg.Issuer, types.ActionIssue, entry.LicenseTypeId)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not hold the issue grant for license type %s", msg.Issuer, entry.LicenseTypeId)
		}
	}

	// Check supply caps up front, aggregating requested counts per license
	// type, so no licenses are issued if any entry would exceed a cap.
	//
	// max_supply bounds licenses *outstanding*, not licenses ever issued: the
	// check is against active_count, which revocation decrements. Revoking
	// therefore returns a slot to the pool, which is what a chargeback on a
	// purchased license needs — the sale is undone, so it must stop consuming
	// supply. issued_count keeps counting lifetime issuance for audit and can
	// exceed max_supply over time; it is deliberately not the cap.
	totals := make(map[string]math.Int)
	for i, entry := range msg.Entries {
		lt, err := ms.k.LicenseTypes.Get(ctx, entry.LicenseTypeId)
		if err != nil {
			return nil, errorsmod.Wrapf(types.ErrLicenseTypeNotFound, "entry %d: license type %s not found", i, entry.LicenseTypeId)
		}

		total, ok := totals[entry.LicenseTypeId]
		if !ok {
			total = math.ZeroInt()
		}
		total = total.Add(math.NewIntFromUint64(entry.Count))
		totals[entry.LicenseTypeId] = total

		if !lt.MaxSupply.IsZero() && lt.ActiveCount.Add(total).GT(lt.MaxSupply) {
			return nil, errorsmod.Wrapf(types.ErrMaxSupplyReached, "entry %d: license type %s: issuing %d would exceed max supply of %s (outstanding: %s)", i, entry.LicenseTypeId, entry.Count, lt.MaxSupply.String(), lt.ActiveCount.String())
		}
	}

	ids := make([]uint64, 0, len(msg.Entries))
	for _, entry := range msg.Entries {
		// Re-read the license type each entry so counts accumulate correctly
		// when multiple entries reference the same type.
		lt, err := ms.k.LicenseTypes.Get(ctx, entry.LicenseTypeId)
		if err != nil {
			return nil, err
		}
		countInt := math.NewIntFromUint64(entry.Count)

		// Issuance creates the holder's account if absent, so a wallet
		// holding only a license can sign its first transaction.
		holderAddr, err := sdk.AccAddressFromBech32(entry.Holder)
		if err != nil {
			return nil, fmt.Errorf("invalid holder address %q: %w", entry.Holder, err)
		}
		ms.k.createAccountIfNotExists(ctx, holderAddr)

		for j := uint64(0); j < entry.Count; j++ {
			id, err := ms.k.nextLicenseID(ctx)
			if err != nil {
				return nil, err
			}

			license := types.License{
				Id:        id,
				Type:      entry.LicenseTypeId,
				Holder:    entry.Holder,
				StartDate: entry.StartDate,
				EndDate:   entry.EndDate,
				Status:    types.StatusActive,
			}

			if err := ms.k.Licenses.Set(ctx, id, license); err != nil {
				return nil, err
			}
			// Written once here and never moved: a license's type is fixed,
			// and this index covers revoked licenses too.
			if err := ms.k.LicensesByType.Set(ctx, collections.Join(entry.LicenseTypeId, id)); err != nil {
				return nil, err
			}
			if err := ms.k.ActiveLicensesByHolder.Set(ctx, collections.Join3(entry.Holder, entry.LicenseTypeId, id)); err != nil {
				return nil, err
			}

			ids = append(ids, id)
		}

		lt.IssuedCount = lt.IssuedCount.Add(countInt)
		lt.ActiveCount = lt.ActiveCount.Add(countInt)
		if err := ms.k.LicenseTypes.Set(ctx, entry.LicenseTypeId, lt); err != nil {
			return nil, err
		}

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			types.EventTypeIssueLicenses,
			sdk.NewAttribute(types.AttributeKeyLicenseTypeID, entry.LicenseTypeId),
			sdk.NewAttribute(types.AttributeKeyHolder, entry.Holder),
			sdk.NewAttribute("count", fmt.Sprintf("%d", entry.Count)),
		))
	}

	return &types.MsgIssueLicensesResponse{Ids: ids}, nil
}

// RevokeLicenses revokes the licenses named by id in the message. Each id
// must be an active license of the message's license type; the signer must
// hold the "revoke" grant for that type. All ids are checked before any
// license is revoked, so no licenses are revoked if any id fails.
func (ms msgServer) RevokeLicenses(ctx context.Context, msg *types.MsgRevokeLicenses) (*types.MsgRevokeLicensesResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	allowed, err := ms.k.can(ctx, msg.Revoker, types.ActionRevoke, msg.LicenseTypeId)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, errorsmod.Wrapf(types.ErrUnauthorized, "%s does not hold the revoke grant for license type %s", msg.Revoker, msg.LicenseTypeId)
	}

	if len(msg.LicenseIds) == 0 {
		return nil, errorsmod.Wrap(types.ErrEmptyLicenseIDs, "license_ids must not be empty")
	}
	if len(msg.LicenseIds) > types.MaxRevokeBatchSize {
		return nil, fmt.Errorf("license ids length %d exceeds max batch size %d", len(msg.LicenseIds), types.MaxRevokeBatchSize)
	}

	// Duplicates are rejected rather than deduped: each occurrence past the
	// first would decrement active_count again below.
	licenses := make([]types.License, 0, len(msg.LicenseIds))
	seen := make(map[uint64]struct{}, len(msg.LicenseIds))
	for _, id := range msg.LicenseIds {
		if _, ok := seen[id]; ok {
			return nil, errorsmod.Wrapf(types.ErrDuplicateLicenseID, "license id %d", id)
		}
		seen[id] = struct{}{}

		license, err := ms.k.Licenses.Get(ctx, id)
		if err != nil {
			if errors.Is(err, collections.ErrNotFound) {
				return nil, errorsmod.Wrapf(types.ErrLicenseNotFound, "license %d not found", id)
			}
			return nil, err
		}
		if license.Type != msg.LicenseTypeId {
			return nil, errorsmod.Wrapf(types.ErrLicenseNotFound, "license %d is of type %s, not %s", id, license.Type, msg.LicenseTypeId)
		}
		if license.Status != types.StatusActive {
			return nil, errorsmod.Wrapf(types.ErrLicenseNotActive, "license %d has status %s", id, license.Status.Short())
		}
		licenses = append(licenses, license)
	}

	revokedDate := sdkCtx.BlockTime().Format("2006-01-02")
	revokedIDs := make([]uint64, 0, len(licenses))

	for _, license := range licenses {
		// end_date is overwritten with the revocation date: it records when
		// the license stopped being valid, whatever expiry it was issued with.
		license.Status = types.StatusRevoked
		license.EndDate = revokedDate

		// LicensesByType is left alone: it lists revoked licenses too.
		if err := ms.k.Licenses.Set(ctx, license.Id, license); err != nil {
			return nil, err
		}
		if err := ms.k.ActiveLicensesByHolder.Remove(ctx, collections.Join3(license.Holder, license.Type, license.Id)); err != nil {
			return nil, err
		}

		revokedIDs = append(revokedIDs, license.Id)
	}

	lt, err := ms.k.LicenseTypes.Get(ctx, msg.LicenseTypeId)
	if err != nil {
		return nil, err
	}
	countInt := math.NewIntFromUint64(uint64(len(revokedIDs)))
	lt.ActiveCount = lt.ActiveCount.Sub(countInt)
	lt.RevokedCount = lt.RevokedCount.Add(countInt)
	if err := ms.k.LicenseTypes.Set(ctx, msg.LicenseTypeId, lt); err != nil {
		return nil, err
	}

	event := sdk.NewEvent(
		types.EventTypeRevokeLicenses,
		sdk.NewAttribute(types.AttributeKeyLicenseTypeID, msg.LicenseTypeId),
		sdk.NewAttribute("count", fmt.Sprintf("%d", len(revokedIDs))),
	)
	for _, id := range revokedIDs {
		event = event.AppendAttributes(sdk.NewAttribute(types.AttributeKeyLicenseID, fmt.Sprintf("%d", id)))
	}
	sdkCtx.EventManager().EmitEvent(event)

	return &types.MsgRevokeLicensesResponse{Ids: revokedIDs}, nil
}

// ---------------------------------------------------------------------------
// Ownership and access grants
// ---------------------------------------------------------------------------

// UpdateParams replaces the module parameters. The signer must be the module
// authority (governance). It is how an owner is first established outside
// genesis, and the recovery path when an owner key is lost or compromised —
// so it deliberately does not require the current owner's cooperation.
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

// TransferOwnership hands the module to a new owner. The current owner signs.
//
// This is the one path by which a non-authority signer writes a parameter, so
// it reads the current set back and replaces only Owner: a transfer must never
// be usable to reset an unrelated parameter to its zero value.
func (ms msgServer) TransferOwnership(ctx context.Context, msg *types.MsgTransferOwnership) (*types.MsgTransferOwnershipResponse, error) {
	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	if err := accesstypes.ValidateCanonicalAddress("new owner", msg.NewOwner); err != nil {
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
// Nothing is ever removed by this message; use RevokeAccess for that. Every
// (action, scope) pair is validated before anything is written, so a
// partially-invalid message grants nothing.
func (ms msgServer) GrantAccess(ctx context.Context, msg *types.MsgGrantAccess) (*types.MsgGrantAccessResponse, error) {
	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	if err := accesstypes.ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return nil, err
	}

	if err := ms.k.Grants.GrantTo(ctx, msg.Grantee, msg.Grants); err != nil {
		return nil, err
	}

	return &types.MsgGrantAccessResponse{}, nil
}

// RevokeAccess removes specific (action, scope) pairs from a grantee. Pairs
// that are not currently granted are silently ignored, so the same revoke can
// be safely re-sent.
func (ms msgServer) RevokeAccess(ctx context.Context, msg *types.MsgRevokeAccess) (*types.MsgRevokeAccessResponse, error) {
	if err := ms.k.requireOwner(ctx, msg.Owner); err != nil {
		return nil, err
	}

	// Validated here as well as in ValidateBasic: a non-canonical grantee is
	// not merely malformed, it names a key this revoke could never match, and
	// RevokeFrom reports a miss as success.
	if err := accesstypes.ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return nil, err
	}

	if err := ms.k.Grants.RevokeFrom(ctx, msg.Grantee, msg.Actions); err != nil {
		return nil, err
	}

	return &types.MsgRevokeAccessResponse{}, nil
}
