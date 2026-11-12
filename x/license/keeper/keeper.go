package keeper

import (
	"context"
	"errors"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"

	accesskeeper "github.com/nodelabs-sdk/nodelabs/access/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

type Keeper struct {
	cdc    codec.BinaryCodec
	logger log.Logger

	// state management
	Schema       collections.Schema
	LicenseTypes collections.Map[string, types.LicenseType]

	// Licenses is keyed by license id alone. Ids come from one chain-wide
	// sequence, so an id identifies a license without its type; the type is
	// carried in the value.
	Licenses collections.Map[uint64, types.License]

	// NextLicenseID is the id that will be assigned to the next issued
	// license, chain-wide across every license type.
	NextLicenseID collections.Item[uint64]

	// LicensesByType indexes (license_type_id, license_id) so licenses of one
	// type can be listed without scanning every license. A license's type
	// never changes, so entries are written once at issuance and never moved
	// or removed — revoked licenses stay listed, matching what the
	// LicensesByType query has always returned.
	LicensesByType collections.KeySet[collections.Pair[string, uint64]]

	// ActiveLicensesByHolder indexes (holder, license_type_id, license_id) for
	// active licenses only: entries are added on issue, moved on transfer, and
	// removed on revoke. Revoked licenses remain in Licenses.
	ActiveLicensesByHolder collections.KeySet[collections.Triple[string, string, uint64]]

	// ActiveLicenseCounts is the denormalized size of each
	// (holder, license_type_id) prefix of ActiveLicensesByHolder, maintained
	// alongside every write to that index. CountActiveLicenses reads it so the
	// network module's activation checks cost the same however many licenses
	// an operator holds. Zero counts are removed.
	ActiveLicenseCounts collections.Map[collections.Pair[string, string], uint64]

	// Params holds the module parameters, including the module owner — the
	// address allowed to grant and revoke access within this module.
	Params collections.Item[types.Params]

	// Grants holds the module's access grants, keyed (grantee, action, scope).
	Grants accesskeeper.Grants

	// accountKeeper creates holder accounts on issuance.
	accountKeeper types.AccountKeeper

	authority string
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeService storetypes.KVStoreService,
	logger log.Logger,
	authority string,
	accountKeeper types.AccountKeeper,
) Keeper {
	logger = logger.With(log.ModuleKey, "x/"+types.ModuleName)

	sb := collections.NewSchemaBuilder(storeService)

	if authority == "" {
		authority = authtypes.NewModuleAddress(govtypes.ModuleName).String()
	}

	// Bound before the keeper literal so the grant store's scope validator can
	// close over the collection itself rather than over a keeper that is still
	// being assembled.
	licenseTypes := collections.NewMap(sb, types.LicenseTypePrefix, "license_types", collections.StringKey, codec.CollValue[types.LicenseType](cdc))

	k := Keeper{
		cdc:    cdc,
		logger: logger,

		LicenseTypes:  licenseTypes,
		Licenses:      collections.NewMap(sb, types.LicensePrefix, "licenses", collections.Uint64Key, codec.CollValue[types.License](cdc)),
		NextLicenseID: collections.NewItem(sb, types.NextLicenseIDPrefix, "next_license_id", collections.Uint64Value),

		LicensesByType:         collections.NewKeySet(sb, types.LicensesByTypePrefix, "licenses_by_type", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		ActiveLicensesByHolder: collections.NewKeySet(sb, types.ActiveLicensesByHolderPrefix, "active_licenses_by_holder", collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.Uint64Key)),
		ActiveLicenseCounts:    collections.NewMap(sb, types.ActiveLicenseCountsPrefix, "active_license_counts", collections.PairKeyCodec(collections.StringKey, collections.StringKey), collections.Uint64Value),

		Params: collections.NewItem(sb, types.ParamsPrefix, "params", codec.CollValue[types.Params](cdc)),
		Grants: accesskeeper.NewGrants(sb, types.GrantsPrefix, "grants", types.ModuleName,
			types.Spec(func(ctx context.Context, scope string) (bool, error) {
				return licenseTypes.Has(ctx, scope)
			})),

		accountKeeper: accountKeeper,

		authority: authority,
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	k.Schema = schema

	return k
}

func (k Keeper) Logger() log.Logger {
	return k.logger
}

func (k Keeper) GetAuthority() string {
	return k.authority
}

// GetLicenseType returns a license type by id and whether it was found.
func (k Keeper) GetLicenseType(ctx context.Context, id string) (types.LicenseType, bool, error) {
	lt, err := k.LicenseTypes.Get(ctx, id)
	if err != nil {
		return types.LicenseType{}, false, nil
	}
	return lt, true, nil
}

// HasLicenseType reports whether license type id exists. It is the narrow
// surface consuming modules gate on when they only need to know a license type
// is real, without depending on the record's shape.
//
// Unlike GetLicenseType, a store failure is returned as an error rather than
// flattened into "not found": callers turn not-found into a validation error,
// and a read failure must not be reported as a missing license type.
func (k Keeper) HasLicenseType(ctx context.Context, id string) (bool, error) {
	has, err := k.LicenseTypes.Has(ctx, id)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return false, err
	}
	return has, nil
}

// GetLicense returns a license by id and whether it was found. Ids are unique
// chain-wide, so no license type is needed; the type is on the returned value.
func (k Keeper) GetLicense(ctx context.Context, id uint64) (types.License, bool, error) {
	l, err := k.Licenses.Get(ctx, id)
	if err != nil {
		return types.License{}, false, nil
	}
	return l, true, nil
}

// can checks whether an address holds an action grant for a license type.
// Scoped actions ("issue", "revoke") pass the type id; module-wide ones (see
// types.UnscopedActions) pass the empty string, which is the only scope they
// can be granted under. A missing grant returns (false, nil); a store error is
// surfaced so the caller can fail the tx instead of silently denying the
// action.
func (k Keeper) can(ctx context.Context, address, action, licenseTypeID string) (bool, error) {
	return k.Grants.HasGrant(ctx, address, action, licenseTypeID)
}

// GetParams returns the module parameters. An unset params item reads as the
// defaults, so a chain that never ran InitGenesis has no owner rather than a
// store error.
func (k Keeper) GetParams(ctx context.Context) (types.Params, error) {
	params, err := k.Params.Get(ctx)
	if errors.Is(err, collections.ErrNotFound) {
		return types.DefaultParams(), nil
	}
	if err != nil {
		return types.Params{}, err
	}
	return params, nil
}

// isOwner checks whether the sender is the module owner. An owner that has
// never been set is surfaced as ErrOwnerNotSet, so callers can tell "not the
// owner" from "no owner configured" — the module fails closed either way.
func (k Keeper) isOwner(ctx context.Context, sender string) (bool, error) {
	params, err := k.GetParams(ctx)
	if err != nil {
		return false, err
	}
	if params.Owner == "" {
		return false, types.ErrOwnerNotSet.Wrapf("no owner is set for module %q", types.ModuleName)
	}
	return params.Owner == sender, nil
}

// requireOwner is the guard every owner-gated handler runs first.
func (k Keeper) requireOwner(ctx context.Context, signer string) error {
	isOwner, err := k.isOwner(ctx, signer)
	if err != nil {
		return err
	}
	if !isOwner {
		return types.ErrUnauthorized.Wrapf("signer %s is not the owner of module %q", signer, types.ModuleName)
	}
	return nil
}

// InitGenesis initializes the module's state from a genesis state. It runs the
// full GenesisState.Validate up front so direct keeper callers (tests, future
// migrations) get the same invariant enforcement as the JSON ValidateGenesis
// path on the AppModule.
func (k *Keeper) InitGenesis(ctx context.Context, data *types.GenesisState) error {
	if err := data.Validate(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, data.Params); err != nil {
		return err
	}

	// License types first: grant scopes name them, and the grant store's scope
	// validator reads them back out of state below.
	for _, lt := range data.LicenseTypes {
		if err := k.LicenseTypes.Set(ctx, lt.Id, lt); err != nil {
			return err
		}
	}

	if err := k.Grants.Init(ctx, data.Grants); err != nil {
		return err
	}

	// The id sequence is genesis state in its own right; it is never derived
	// from the licenses below or from the stats counters.
	if err := k.NextLicenseID.Set(ctx, data.NextLicenseId); err != nil {
		return err
	}

	for _, license := range data.Licenses {
		if err := k.Licenses.Set(ctx, license.Id, license); err != nil {
			return err
		}
		// The by-type index covers active and revoked alike.
		if err := k.LicensesByType.Set(ctx, collections.Join(license.Type, license.Id)); err != nil {
			return err
		}
		// The holder index tracks active licenses only.
		if license.Status == types.StatusActive {
			if err := k.AddActiveLicense(ctx, license.Holder, license.Type, license.Id); err != nil {
				return err
			}
		}
	}

	return nil
}

// ExportGenesis exports the module's state to a genesis state.
func (k *Keeper) ExportGenesis(ctx context.Context) *types.GenesisState {
	var licenseTypes []types.LicenseType
	if err := k.LicenseTypes.Walk(ctx, nil, func(_ string, lt types.LicenseType) (bool, error) {
		licenseTypes = append(licenseTypes, lt)
		return false, nil
	}); err != nil {
		panic(err)
	}

	// Licenses export in ascending id order: Uint64Key is big-endian, so byte
	// order is numeric order.
	var licenses []types.License
	if err := k.Licenses.Walk(ctx, nil, func(_ uint64, l types.License) (bool, error) {
		licenses = append(licenses, l)
		return false, nil
	}); err != nil {
		panic(err)
	}

	nextID, err := k.NextLicenseID.Get(ctx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			panic(err)
		}
		nextID = types.FirstLicenseID
	}

	params, err := k.GetParams(ctx)
	if err != nil {
		panic(err)
	}

	grants, err := k.Grants.Export(ctx)
	if err != nil {
		panic(err)
	}

	return &types.GenesisState{
		LicenseTypes:  licenseTypes,
		Licenses:      licenses,
		NextLicenseId: nextID,
		Params:        params,
		Grants:        grants,
	}
}

// createAccountIfNotExists creates a BaseAccount for addr if none exists,
// so license holders can sign gasless transactions without a prior funding
// transfer. Pubkeys are set lazily by
// the stock SetPubKeyDecorator on the account's first signed tx.
func (k Keeper) createAccountIfNotExists(ctx context.Context, addr sdk.AccAddress) {
	if k.accountKeeper.HasAccount(ctx, addr) {
		return
	}
	acc := k.accountKeeper.NewAccountWithAddress(ctx, addr)
	k.accountKeeper.SetAccount(ctx, acc)
}

// CountActiveLicenses returns the number of active licenses holder holds
// across the given license types. It reads the denormalized
// ActiveLicenseCounts, so its cost is one read per license type regardless of
// holdings.
//
// stopAt is ignored and the exact count is always returned. It bounded an
// index walk before the counts existed, and stays in the signature so the
// cross-module LicenseKeeper interface is unchanged for consumers.
//
// "Active" means "not revoked": the module never enforces EndDate, so an
// expired-but-unrevoked license still counts. License types meant for
// counting should be issued with an empty end_date (revocation-only
// lifecycle).
func (k Keeper) CountActiveLicenses(ctx context.Context, holder string, licenseTypes []string, _ uint64) (uint64, error) {
	var count uint64
	for _, typeID := range licenseTypes {
		n, err := k.getActiveLicenseCount(ctx, holder, typeID)
		if err != nil {
			return 0, err
		}
		count += n
	}
	return count, nil
}

// AddActiveLicense inserts a license into ActiveLicensesByHolder and bumps the
// holder's denormalized count. Every insert into the index must go through
// here so the two never diverge.
func (k Keeper) AddActiveLicense(ctx context.Context, holder, typeID string, id uint64) error {
	if err := k.ActiveLicensesByHolder.Set(ctx, collections.Join3(holder, typeID, id)); err != nil {
		return err
	}
	n, err := k.getActiveLicenseCount(ctx, holder, typeID)
	if err != nil {
		return err
	}
	return k.ActiveLicenseCounts.Set(ctx, collections.Join(holder, typeID), n+1)
}

// removeActiveLicense is the inverse of AddActiveLicense.
func (k Keeper) removeActiveLicense(ctx context.Context, holder, typeID string, id uint64) error {
	has, err := k.ActiveLicensesByHolder.Has(ctx, collections.Join3(holder, typeID, id))
	if err != nil || !has {
		return err
	}
	if err := k.ActiveLicensesByHolder.Remove(ctx, collections.Join3(holder, typeID, id)); err != nil {
		return err
	}
	n, err := k.getActiveLicenseCount(ctx, holder, typeID)
	if err != nil {
		return err
	}
	if n <= 1 {
		return k.ActiveLicenseCounts.Remove(ctx, collections.Join(holder, typeID))
	}
	return k.ActiveLicenseCounts.Set(ctx, collections.Join(holder, typeID), n-1)
}

func (k Keeper) getActiveLicenseCount(ctx context.Context, holder, typeID string) (uint64, error) {
	n, err := k.ActiveLicenseCounts.Get(ctx, collections.Join(holder, typeID))
	if errors.Is(err, collections.ErrNotFound) {
		return 0, nil
	}
	return n, err
}

// RebuildActiveLicenseCounts recomputes ActiveLicenseCounts from the
// ActiveLicensesByHolder index, discarding whatever was stored. Used by the
// store migration that introduced the counts.
func (k Keeper) RebuildActiveLicenseCounts(ctx context.Context) error {
	if err := k.ActiveLicenseCounts.Clear(ctx, nil); err != nil {
		return err
	}
	type holderType struct{ holder, typeID string }
	counts := make(map[holderType]uint64)
	var order []holderType
	if err := k.ActiveLicensesByHolder.Walk(ctx, nil, func(key collections.Triple[string, string, uint64]) (bool, error) {
		ht := holderType{key.K1(), key.K2()}
		if _, ok := counts[ht]; !ok {
			order = append(order, ht)
		}
		counts[ht]++
		return false, nil
	}); err != nil {
		return err
	}
	// order follows the index's key order, so writes are deterministic.
	for _, ht := range order {
		if err := k.ActiveLicenseCounts.Set(ctx, collections.Join(ht.holder, ht.typeID), counts[ht]); err != nil {
			return err
		}
	}
	return nil
}

// nextLicenseID returns the id to assign to the next license and advances the
// chain-wide sequence. An unset sequence starts at FirstLicenseID, so the
// first license issued on a chain that never ran InitGenesis still gets 1
// rather than 0. Store errors other than "not found" are surfaced: silently
// restarting the sequence would hand out an id that is already in use.
func (k Keeper) nextLicenseID(ctx context.Context) (uint64, error) {
	id, err := k.NextLicenseID.Get(ctx)
	switch {
	case errors.Is(err, collections.ErrNotFound):
		id = types.FirstLicenseID
	case err != nil:
		return 0, err
	}
	if err := k.NextLicenseID.Set(ctx, id+1); err != nil {
		return 0, err
	}
	return id, nil
}
