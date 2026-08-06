package types

import (
	"fmt"
	"time"

	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// ValidateDates validates start_date and end_date strings in YYYY-MM-DD form.
// start_date is required; end_date is optional and, if present, must not be
// before start_date.
func ValidateDates(startDate, endDate string) error {
	if startDate == "" {
		return fmt.Errorf("start_date is required")
	}
	sd, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return fmt.Errorf("invalid start_date %q: must be YYYY-MM-DD format", startDate)
	}
	if endDate != "" {
		ed, err := time.Parse("2006-01-02", endDate)
		if err != nil {
			return fmt.Errorf("invalid end_date %q: must be YYYY-MM-DD format", endDate)
		}
		if ed.Before(sd) {
			return fmt.Errorf("end_date %s must not be before start_date %s", endDate, startDate)
		}
	}
	return nil
}

var (
	_ sdk.Msg = &MsgCreateLicenseType{}
	_ sdk.Msg = &MsgIssueLicenses{}
	_ sdk.Msg = &MsgRevokeLicenses{}
	_ sdk.Msg = &MsgUpdateLicenseType{}
	_ sdk.Msg = &MsgUpdateParams{}
	_ sdk.Msg = &MsgTransferOwnership{}
	_ sdk.Msg = &MsgGrantAccess{}
	_ sdk.Msg = &MsgRevokeAccess{}
)

func (msg *MsgUpdateParams) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("authority", msg.Authority); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	return msg.Params.Validate()
}

func (msg *MsgTransferOwnership) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	// new_owner becomes Params.Owner, which every owner check compares by
	// string equality — a non-canonical alias there would be an owner nobody
	// can authenticate as.
	return accesstypes.ValidateCanonicalAddress("new owner", msg.NewOwner)
}

func (msg *MsgGrantAccess) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	// The grantee is a state key. A non-canonical alias would be a second
	// identity for the same account: exercisable by its holder, but invisible
	// to a revoke or an audit query using the canonical form.
	if err := accesstypes.ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return err
	}
	return accesstypes.ValidateGrantEntries(msg.Grants)
}

func (msg *MsgRevokeAccess) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	if err := accesstypes.ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return err
	}
	return accesstypes.ValidateRevokePairs(msg.Actions)
}

func (msg *MsgCreateLicenseType) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("creator", msg.Creator); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	if msg.Id == "" {
		return ErrEmptyLicenseTypeID
	}
	return ValidateMaxSupply(msg.MaxSupply)
}

func (msg *MsgUpdateLicenseType) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	if msg.Id == "" {
		return ErrEmptyLicenseTypeID
	}
	return nil
}

func ValidateMaxSupply(v math.Int) error {
	if v.IsNil() {
		return ErrInvalidMaxSupply.Wrap("max_supply must be set")
	}
	if v.IsNegative() {
		return ErrInvalidMaxSupply.Wrapf("max_supply must not be negative, got %s", v.String())
	}
	return nil
}

func (msg *MsgIssueLicenses) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("issuer", msg.Issuer); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	if len(msg.Entries) == 0 {
		return ErrEmptyBatchEntries
	}
	if len(msg.Entries) > MaxIssueBatchSize {
		return fmt.Errorf("entries length %d exceeds max batch size %d", len(msg.Entries), MaxIssueBatchSize)
	}
	for i, entry := range msg.Entries {
		if entry.LicenseTypeId == "" {
			return ErrEmptyLicenseTypeID.Wrapf("entry %d", i)
		}
		if err := accesstypes.ValidateCanonicalAddress("holder", entry.Holder); err != nil {
			return ErrEmptyHolder.Wrapf("entry %d: %s", i, err)
		}
		if entry.Count == 0 {
			return ErrInvalidCount.Wrapf("entry %d: count must be greater than zero", i)
		}
	}
	return nil
}

func (msg *MsgRevokeLicenses) ValidateBasic() error {
	if err := accesstypes.ValidateCanonicalAddress("revoker", msg.Revoker); err != nil {
		return ErrInvalidSigner.Wrap(err.Error())
	}
	if msg.LicenseTypeId == "" {
		return ErrEmptyLicenseTypeID
	}
	if len(msg.LicenseIds) == 0 {
		return ErrEmptyLicenseIDs
	}
	if len(msg.LicenseIds) > MaxRevokeBatchSize {
		return fmt.Errorf("license ids length %d exceeds max batch size %d", len(msg.LicenseIds), MaxRevokeBatchSize)
	}
	seen := make(map[uint64]struct{}, len(msg.LicenseIds))
	for _, id := range msg.LicenseIds {
		if id < FirstLicenseID {
			return ErrLicenseNotFound.Wrapf("license id %d is not a valid id", id)
		}
		if _, ok := seen[id]; ok {
			return ErrDuplicateLicenseID.Wrapf("license id %d", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
