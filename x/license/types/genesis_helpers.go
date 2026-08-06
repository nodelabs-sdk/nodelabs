package types

import (
	"fmt"
	"time"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		LicenseTypes:  []LicenseType{},
		Licenses:      []License{},
		NextLicenseId: FirstLicenseID,
		Params:        DefaultParams(),
		Grants:        []accesstypes.Grant{},
	}
}

func (gs GenesisState) Validate() error {
	typeIDs := make(map[string]struct{})
	for _, lt := range gs.LicenseTypes {
		if _, exists := typeIDs[lt.Id]; exists {
			return fmt.Errorf("duplicate license type id: %s", lt.Id)
		}
		typeIDs[lt.Id] = struct{}{}

		if err := ValidateMaxSupply(lt.MaxSupply); err != nil {
			return fmt.Errorf("license type %s: %w", lt.Id, err)
		}
		if err := validateNonNegativeCounter(lt.IssuedCount, "issued_count"); err != nil {
			return fmt.Errorf("license type %s: %w", lt.Id, err)
		}
		if err := validateNonNegativeCounter(lt.ActiveCount, "active_count"); err != nil {
			return fmt.Errorf("license type %s: %w", lt.Id, err)
		}
		if err := validateNonNegativeCounter(lt.RevokedCount, "revoked_count"); err != nil {
			return fmt.Errorf("license type %s: %w", lt.Id, err)
		}
	}

	// Pass 1: detect duplicate license ids. Ids are unique chain-wide, so two
	// licenses of *different* types may not share one either. Done before
	// per-license validation so the duplicate error fires regardless of the
	// offending record's date or holder fields.
	licenseIDs := make(map[uint64]struct{}, len(gs.Licenses))
	for _, l := range gs.Licenses {
		if _, exists := licenseIDs[l.Id]; exists {
			return fmt.Errorf("duplicate license id %d", l.Id)
		}
		licenseIDs[l.Id] = struct{}{}
	}

	// Pass 2: per-license validation + per-type tally of active/revoked and
	// the highest id seen anywhere (for the sequence invariant below).
	activeByType := make(map[string]uint64)
	revokedByType := make(map[string]uint64)
	var maxID uint64
	for _, l := range gs.Licenses {
		if _, exists := typeIDs[l.Type]; !exists {
			return fmt.Errorf("license (type=%s, id=%d) references unknown license type", l.Type, l.Id)
		}

		if _, err := sdk.AccAddressFromBech32(l.Holder); err != nil {
			return fmt.Errorf("license (type=%s, id=%d) has invalid holder address %q: %w", l.Type, l.Id, l.Holder, err)
		}

		switch l.Status {
		case StatusActive:
			activeByType[l.Type]++
			if err := ValidateDates(l.StartDate, l.EndDate); err != nil {
				return fmt.Errorf("license (type=%s, id=%d): %w", l.Type, l.Id, err)
			}
		case StatusRevoked:
			revokedByType[l.Type]++
			if l.EndDate == "" {
				return fmt.Errorf("license (type=%s, id=%d) is revoked but has no end_date", l.Type, l.Id)
			}
			// A revoked license's end_date holds the revocation date, which may
			// precede start_date (revoked before it ever became valid), so the
			// dates are checked for form only, not order.
			if _, err := time.Parse("2006-01-02", l.StartDate); err != nil {
				return fmt.Errorf("license (type=%s, id=%d) has invalid start_date %q: must be YYYY-MM-DD format", l.Type, l.Id, l.StartDate)
			}
			if _, err := time.Parse("2006-01-02", l.EndDate); err != nil {
				return fmt.Errorf("license (type=%s, id=%d) has invalid end_date %q: must be YYYY-MM-DD format", l.Type, l.Id, l.EndDate)
			}
		default:
			return fmt.Errorf("license (type=%s, id=%d) has invalid status %q", l.Type, l.Id, l.Status.String())
		}

		if l.Id > maxID {
			maxID = l.Id
		}
	}

	// Pass 3: per-type counter invariants must agree with the license set.
	for _, lt := range gs.LicenseTypes {
		wantActive := math.NewIntFromUint64(activeByType[lt.Id])
		wantRevoked := math.NewIntFromUint64(revokedByType[lt.Id])
		wantIssued := wantActive.Add(wantRevoked)

		if !lt.ActiveCount.Equal(wantActive) {
			return fmt.Errorf("license type %s: active_count %s does not match %s active license(s) in genesis", lt.Id, lt.ActiveCount.String(), wantActive.String())
		}
		if !lt.RevokedCount.Equal(wantRevoked) {
			return fmt.Errorf("license type %s: revoked_count %s does not match %s revoked license(s) in genesis", lt.Id, lt.RevokedCount.String(), wantRevoked.String())
		}
		if !lt.IssuedCount.Equal(wantIssued) {
			return fmt.Errorf("license type %s: issued_count %s does not match %s license(s) in genesis (active+revoked)", lt.Id, lt.IssuedCount.String(), wantIssued.String())
		}
	}

	// Pass 4: the chain-wide id sequence must be set and must be past every
	// imported id — otherwise the next issuance would overwrite an existing
	// license. Kept last so the more specific errors above fire first.
	if gs.NextLicenseId < FirstLicenseID {
		return fmt.Errorf("next_license_id must be at least %d, got %d", FirstLicenseID, gs.NextLicenseId)
	}
	if gs.NextLicenseId <= maxID {
		return fmt.Errorf("next_license_id %d is not above the highest license id %d", gs.NextLicenseId, maxID)
	}

	// Pass 5: params and access grants.
	if err := gs.Params.Validate(); err != nil {
		return err
	}
	if err := accesstypes.ValidateGrants(gs.Grants); err != nil {
		return err
	}
	// Grants and the license types they scope to now live in one document, so
	// the reference can be checked here rather than only at import time.
	for i, g := range gs.Grants {
		if err := validateGrantScope(g, typeIDs); err != nil {
			return fmt.Errorf("grant %d: %w", i, err)
		}
	}

	return nil
}

// validateGrantScope checks a genesis grant against the module's action rules:
// module-wide actions carry no scope, and every other action must name a
// license type present in this genesis.
func validateGrantScope(g accesstypes.Grant, typeIDs map[string]struct{}) error {
	spec := Spec(nil)
	if !spec.HasAction(g.Action) {
		return fmt.Errorf("action %q is not part of this module's vocabulary", g.Action)
	}
	if spec.IsUnscoped(g.Action) {
		if g.Scope != "" {
			return fmt.Errorf("action %q is module-wide: scope must be empty, got %q", g.Action, g.Scope)
		}
		return nil
	}
	if g.Scope == "" {
		return fmt.Errorf("action %q must name a license type", g.Action)
	}
	if _, exists := typeIDs[g.Scope]; !exists {
		return fmt.Errorf("action %q references unknown license type %q", g.Action, g.Scope)
	}
	return nil
}

// validateNonNegativeCounter rejects nil or negative math.Int counters that
// would otherwise panic on later arithmetic.
func validateNonNegativeCounter(v math.Int, name string) error {
	if v.IsNil() {
		return fmt.Errorf("%s must be set", name)
	}
	if v.IsNegative() {
		return fmt.Errorf("%s must not be negative, got %s", name, v.String())
	}
	return nil
}
