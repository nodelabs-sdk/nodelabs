// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @dev The default LicenseI precompile address. The chain may register the
///      precompile at a different address; consult chain documentation.
address constant LICENSE_PRECOMPILE_ADDRESS = 0x6E6F64656c616273000000000000000000000001;

/// @dev The LicenseI contract's instance.
LicenseI constant LICENSE_CONTRACT = LicenseI(LICENSE_PRECOMPILE_ADDRESS);

/// @dev LicenseType describes a class of issuable licenses. The address that
///      created the type is not recorded: creation is gated on the module-wide
///      `type.create` grant, which confers no continuing authority over
///      the resulting type.
struct LicenseType {
    string id;
    bool transferrable;
    uint256 maxSupply;
    uint256 issuedCount;
    uint256 activeCount;
    uint256 revokedCount;
}

/// @dev License is a single issued license. endDate is the issued expiry
///      ("" = none) until revocation, which overwrites it with the revocation
///      date (and sets status to "revoked").
struct License {
    uint64 id;
    string typeId;
    address holder;
    string startDate;
    string endDate;
    string status;
}

/// @dev IssueLicenseEntry is a single issuance within an issueLicenses call.
struct IssueLicenseEntry {
    string licenseTypeId;
    address holder;
    string startDate;
    string endDate;
    uint64 count;
}

/// @author Nodelabs
/// @title Licenses Precompile Contract
/// @dev Exposes the x/license module to EVM smart contracts. Ownership and
///      access grants are managed by the x/license module itself — the owner
///      is a module parameter — and are not exposed through this precompile.
interface LicenseI {
    // ---------------------------------------------------------------------
    // Events
    // ---------------------------------------------------------------------

    /// @dev Emitted when a new license type is created.
    event LicenseTypeCreated(string indexed id, bool transferrable, uint256 maxSupply);

    /// @dev Emitted when a license type is updated.
    event LicenseTypeUpdated(string indexed id, bool transferrable);

    /// @dev Emitted when one or more licenses of a single type are issued to a holder.
    event LicenseIssued(
        address indexed issuer,
        address indexed holder,
        string licenseTypeId,
        uint64 count
    );

    /// @dev Emitted when one or more licenses are revoked.
    event LicenseRevoked(
        address indexed revoker,
        string licenseTypeId,
        uint64[] licenseIds
    );

    // ---------------------------------------------------------------------
    // Transactions
    // ---------------------------------------------------------------------

    /// @dev Create a new license type. Caller must hold the module-wide
    ///      `type.create` grant; owning the module is not
    ///      sufficient on its own.
    function createLicenseType(
        string calldata id,
        bool transferrable,
        uint256 maxSupply
    ) external returns (bool success);

    /// @dev Update an existing license type's transferrability. Max supply is
    ///      fixed at creation. Caller must be the module owner.
    function updateLicenseType(
        string calldata id,
        bool transferrable
    ) external returns (bool success);

    /// @dev Issue licenses for each entry. Each entry names its own license
    ///      type, holder, dates, and count; the caller must hold the `issue`
    ///      grant for every referenced license type. Dates are formatted
    ///      as YYYY-MM-DD. Returned ids are flattened in entry order.
    function issueLicenses(
        IssueLicenseEntry[] calldata entries
    ) external returns (uint64[] memory ids);

    /// @dev Revoke the licenses with the given ids. Each id must be an active
    ///      license of the given type. Caller must hold the `revoke` grant.
    function revokeLicenses(
        string calldata licenseTypeId,
        uint64[] calldata licenseIds
    ) external returns (uint64[] memory ids);

    // ---------------------------------------------------------------------
    // Queries
    // ---------------------------------------------------------------------

    /// @dev Returns a single license type by id. Reverts if not found.
    function licenseType(string calldata id) external view returns (LicenseType memory);

    /// @dev Returns all license types.
    function licenseTypes() external view returns (LicenseType[] memory);

    /// @dev Returns a single license by id. Ids are unique chain-wide across
    ///      license types. Reverts if not found.
    function license(uint64 id) external view returns (License memory);

    /// @dev Returns every license across all license types, active and revoked.
    function licenses() external view returns (License[] memory);

    /// @dev Returns all licenses of the given type.
    function licensesByType(string calldata typeId) external view returns (License[] memory);

    /// @dev Returns all licenses held by `holder`.
    function licensesByHolder(address holder) external view returns (License[] memory);

    /// @dev Returns all licenses of a given type held by `holder`.
    function licensesByHolderAndType(
        address holder,
        string calldata typeId
    ) external view returns (License[] memory);
}
