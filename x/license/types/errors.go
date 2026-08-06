package types

import "cosmossdk.io/errors"

var (
	ErrInvalidSigner       = errors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrLicenseTypeNotFound = errors.Register(ModuleName, 1101, "license type not found")
	ErrLicenseTypeExists   = errors.Register(ModuleName, 1102, "license type already exists")
	ErrMaxSupplyReached    = errors.Register(ModuleName, 1103, "license type max supply reached")
	ErrLicenseNotFound     = errors.Register(ModuleName, 1104, "license not found")
	ErrUnauthorized        = errors.Register(ModuleName, 1107, "signer lacks the required grant")
	ErrOwnerNotSet         = errors.Register(ModuleName, 1117, "module owner is not set")
	ErrEmptyLicenseTypeID  = errors.Register(ModuleName, 1111, "license type id cannot be empty")
	ErrEmptyHolder         = errors.Register(ModuleName, 1112, "holder address cannot be empty")
	ErrEmptyBatchEntries   = errors.Register(ModuleName, 1113, "batch entries cannot be empty")
	ErrInvalidMaxSupply    = errors.Register(ModuleName, 1115, "invalid max_supply")
	ErrInvalidCount        = errors.Register(ModuleName, 1116, "invalid count")
	ErrEmptyLicenseIDs     = errors.Register(ModuleName, 1118, "license ids cannot be empty")
	ErrDuplicateLicenseID  = errors.Register(ModuleName, 1119, "duplicate license id")
	ErrLicenseNotActive    = errors.Register(ModuleName, 1120, "license is not active")
)
