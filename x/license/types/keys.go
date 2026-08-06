package types

import (
	"cosmossdk.io/collections"
)

const (
	ModuleName   = "license"
	StoreKey     = ModuleName
	RouterKey    = ModuleName
	QuerierRoute = ModuleName
)

// Short aliases for the generated enum constants.
const (
	StatusActive  = LicenseStatus_LICENSE_STATUS_ACTIVE
	StatusRevoked = LicenseStatus_LICENSE_STATUS_REVOKED
)

// Action names the license module's access vocabulary. Grants are scoped per
// license type id, except for the actions listed in UnscopedActions.
const (
	ActionIssue  = "issue"
	ActionRevoke = "revoke"

	// ActionCreateType authorizes creating new license types. It is
	// module-wide rather than scoped to a type id: the type it would name does
	// not exist at the time the grant is made.
	ActionCreateType = "type.create"
)

// ValidActions is the module's full action vocabulary.
var ValidActions = []string{ActionIssue, ActionRevoke, ActionCreateType}

// UnscopedActions are the entries in ValidActions granted module-wide. Grants
// for these carry the empty scope; every other action must name an existing
// license type id.
var UnscopedActions = []string{ActionCreateType}

// Short returns the lowercase boundary form of a license status ("active",
// "revoked"), or the raw enum name for unknown values.
func (s LicenseStatus) Short() string {
	switch s {
	case StatusActive:
		return "active"
	case StatusRevoked:
		return "revoked"
	default:
		return s.String()
	}
}

// FirstLicenseID is the id assigned to the first license issued on a chain,
// and so the lowest valid license id. Ids start at 1 rather than 0 so that a
// zero id is always invalid rather than being indistinguishable from an unset
// field in a message, an ABI call, or a REST path.
const FirstLicenseID uint64 = 1

// MaxIssueBatchSize bounds the number of entries in a single
// MsgIssueLicenses. Per-tx work is otherwise only bounded by the
// CometBFT tx-size limit; this gives a clean error before the keeper
// starts iterating a pathologically large batch.
const MaxIssueBatchSize = 100

// MaxRevokeBatchSize bounds the number of license ids in a single
// MsgRevokeLicenses, for the same reason as MaxIssueBatchSize.
const MaxRevokeBatchSize = 100

var (
	LicenseTypePrefix = collections.NewPrefix(1)
	LicensePrefix     = collections.NewPrefix(2)
	// NextLicenseIDPrefix holds the single chain-wide id sequence. It was a
	// per-license-type map under the same prefix.
	NextLicenseIDPrefix = collections.NewPrefix(3)
	ParamsPrefix        = collections.NewPrefix(4)
	// GrantsPrefix holds the module's access grants, keyed
	// (grantee, action, scope).
	GrantsPrefix = collections.NewPrefix(5)

	// Index prefixes
	ActiveLicensesByHolderPrefix = collections.NewPrefix(10)
	LicensesByTypePrefix         = collections.NewPrefix(11)
)
