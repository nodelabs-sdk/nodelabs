# Licenses Module

The `x/license` module provides on-chain license management for Cosmos SDK chains. It allows a module owner to define license types and delegated addresses to issue and revoke licenses.

## Overview

- **License Types** define templates (e.g. `node.license`, `validator.license`) with optional max supply and a declarative transferrability flag
- **Licenses** are individual instances issued to holders with start/end dates and active/revoked status
- **Ownership and access grants** live in this module: the owner is the `owner` parameter, and per-type `issue`/`revoke` grants plus a module-wide `type.create` grant are stored alongside the licenses themselves. The owner grants these rights rather than holding them implicitly. The grant machinery is the shared [`x/access`](../access) library
- **An EVM precompile** exposes the same transactions and queries to Solidity contracts, routed through the same handlers
- **Downstream consumers** read licenses through a narrow keeper surface — [`x/network`](../network/README.md) derives its per-node-type activation limits from a holder's active license count

## Installation

### As a dependency in another Cosmos SDK chain

```bash
go get github.com/nodelabs-sdk/nodelabs
```

### Wiring into app.go (manual)

```go
import (
    license "github.com/nodelabs-sdk/nodelabs/x/license"
    licensekeeper "github.com/nodelabs-sdk/nodelabs/x/license/keeper"
    licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)
```

1. Add the store key:

```go
keys := storetypes.NewKVStoreKeys(
    // ... existing keys
    licensetypes.StoreKey,
)
```

2. Create the keeper. It consumes the account keeper (holder accounts are
   created on issuance), so it is constructed after it:

```go
app.LicenseKeeper = licensekeeper.NewKeeper(
    appCodec,
    runtime.NewKVStoreService(keys[licensetypes.StoreKey]),
    logger,
    authAddr, // governance authority address
    app.AccountKeeper,
)
```

3. Register the module:

```go
app.ModuleManager = module.NewManager(
    // ... existing modules
    license.NewAppModule(appCodec, app.LicenseKeeper),
)
```

4. Add to genesis ordering. Grants live in this module's own genesis and are
   imported after its license types, so no cross-module ordering constraint
   applies:

```go
genesisModuleOrder := []string{
    // ... existing modules
    licensetypes.ModuleName,
}
```

### Wiring via depinject

The module supports dependency injection. Add the module proto config to your app config and import the package:

```go
import _ "github.com/nodelabs-sdk/nodelabs/x/license"
```

The `init()` function in `depinject.go` automatically registers the module. The
`ProvideModule` function resolves the codec, store service, and account keeper
from the DI container.

## Concepts

### Module Owner

The module owner is the `owner` parameter. It is the only address that can:
- Update license types
- Grant and revoke access (`MsgGrantAccess`, `MsgRevokeAccess`)
- Hand the module over (`MsgTransferOwnership`)

Ownership confers no other rights. Creating license types is gated on the
`type.create` grant exactly as issuing is gated on `issue`, so a new chain's
first step after setting the owner is for that owner to grant `type.create` —
to itself, to a separate admin address, or both.

The owner is empty until it is set, and while empty the module fails closed:
every owner-gated message returns `ErrOwnerNotSet`, which callers can tell
apart from "you are not the owner". It is set in genesis or by governance
(`MsgUpdateParams`, which is also the recovery path for a lost or compromised
key), and can be handed off directly by the current owner
(`MsgTransferOwnership`).

`MsgTransferOwnership` is the one message that lets a non-authority signer
write a parameter. It reads the current parameter set back and replaces only
`owner`, so a handoff can never reset an unrelated parameter.

### License Types

A license type is a template with:

| Field | Description |
|-------|-------------|
| `id` | Unique string identifier (e.g. `node.license`) |
| `transferrable` | Declares whether licenses of this type may change hands. This module has no transfer message, so nothing on chain reads it — it is metadata for consumers to enforce |
| `max_supply` | Maximum number of *outstanding* licenses, checked against `active_count`. `0` = unlimited |
| `issued_count` | Lifetime licenses issued. May exceed `max_supply`, since revoking frees a slot |
| `active_count` | Currently active licenses. This is what `max_supply` caps |
| `revoked_count` | Licenses revoked to date |

The address that created a type is **not recorded**. The `type.create` grant is
the whole authorization for creating one, and it confers no continuing
authority over the result — downstream modules gate on their own grants, not on
who created a license type. `x/network`, for instance, gates node type
registration on its `nodetype.create` grant and only checks that the license
type exists.

### Licenses

Each license is an instance of a license type:

| Field | Description |
|-------|-------------|
| `id` | Auto-incremented uint64, unique chain-wide across all license types |
| `type` | The license type ID this belongs to |
| `holder` | Bech32 address of the current holder |
| `start_date` | Start date in `YYYY-MM-DD` format |
| `end_date` | End date in `YYYY-MM-DD` format (empty = no expiry); keeps its issued value, revocation never modifies it |
| `status` | `LicenseStatus` enum: `active` or `revoked` |
| `revoked_date` | Block date of revocation in `YYYY-MM-DD` format; empty unless revoked |

Licenses are stored under their `id` alone and never deleted; revocation flips
`status` and stamps `revoked_date`, leaving the issued `end_date` intact.
Because ids are unique chain-wide, **a bare id is a complete handle** — the
single-license query takes only an id, and the type comes off the stored
record.

Two secondary indexes support the listing queries:

- `(type, id)` lists a type's licenses, active and revoked alike. A license's
  type never changes, so entries are written once at issuance and never moved
  or removed.
- `(holder, type, id)` tracks **active** licenses only — written on issue,
  removed on revoke. A license's holder never changes, so no entry is ever
  moved. This powers the holder queries and the revoke-most-recent-first walk.

The next-id sequence is a single chain-wide counter and is its own piece of
state (exported in genesis as `next_license_id`), independent of the
`issued_count` stats counter on the license type. Ids start at 1, so `0` is
never a valid license id. Ids are never reused: revoking frees a `max_supply`
slot but not an id, so a reissued license always gets a fresh number.

### Actions

The module's action vocabulary is `issue`, `revoke`, and `type.create`. The
owner delegates `issue`/`revoke` per license type:

```bash
nodelabsd tx license grant-access nodelabs1admin... issue,revoke node.license,validator.license --from owner
nodelabsd tx license revoke-access nodelabs1admin... issue:node.license --from owner
nodelabsd query license grants-by-grantee nodelabs1admin...
```

Each grant's scope must refer to an existing license type. Wildcards are not
supported — grants must explicitly name each license type. The keeper answers
"may X issue Y?" with a single point-read via
`k.Grants.HasGrant(ctx, addr, "issue", licenseTypeID)`.

`type.create` is the exception. It authorizes creating license types, so there
is no existing id to scope it to — the module declares it **unscoped**, and it
is granted with `-` in place of a scope list:

```bash
nodelabsd tx license grant-access nodelabs1admin... type.create - --from owner
```

Because one invocation applies the same scopes to every action it names,
`type.create` cannot be granted in the same command as `issue`/`revoke`.
Supplying a scope for it is rejected at grant time, so the grant only ever
exists under the empty scope and the keeper checks it as
`k.Grants.HasGrant(ctx, addr, "type.create", "")`.

Query the vocabulary, including which actions are unscoped, with:

```bash
nodelabsd query license actions
```

## Messages

### MsgCreateLicenseType
Create a new license type. Signer must hold the module-wide `type.create`
grant; owning the module is not sufficient on its own.

```bash
# One-time, from the module owner:
nodelabsd tx license grant-access nodelabs1admin... type.create - --from owner

# max_supply is a flag, not a positional arg; it defaults to 0 (unlimited).
nodelabsd tx license create-license-type node.license true --max-supply 1000 --from admin
```

### MsgUpdateLicenseType
Update an existing license type's `transferrable` flag. `max_supply` is fixed
at creation and cannot be changed. The flag is declarative
— this module has no transfer message — so changing it signals intent to
consumers rather than altering any on-chain behaviour.

```bash
nodelabsd tx license update-license-type node.license true --from owner
```

### MsgIssueLicenses
Issue licenses in a single transaction. Each entry carries its own license
type, holder, dates, and count, so one message can issue to multiple holders
across multiple license types. Signer must hold the `issue` grant for every
referenced license type. Returned ids are flattened in entry order.

```bash
nodelabsd tx license issue-licenses \
  node.license:nodelabs1aaa...:1:2026-01-01:2027-01-01 \
  validator.license:nodelabs1bbb...:3:2026-01-01 \
  --from admin
```

Each entry is `license_type_id:holder:count:start_date[:end_date]`. A message
carries at most `MaxIssueBatchSize` (100) entries.

All entries are validated — grants, addresses, dates, counts — and supply
caps are checked with the requested counts aggregated per license type, before
any license is issued, so a message that would breach a cap issues nothing.

Issuance **creates the holder's account** if it does not exist, so a wallet
holding only a license can sign its first transaction (e.g. the gasless
activation-key authorization in [`x/network`](../network/README.md)) without a
prior funding transfer.

### MsgRevokeLicenses
Revoke active licenses for a holder, most recently issued first. Sets status to `revoked` and records the current block date as `revoked_date`; the issued `end_date` is left unchanged. Signer must hold the `revoke` grant.

```bash
nodelabsd tx license revoke-licenses node.license nodelabs1abc... 2 --from admin
```

## Queries

All queries are available via gRPC, REST, and CLI (auto-generated via autocli).

| Query | Description | CLI |
|-------|-------------|-----|
| `LicenseType` | Single license type by ID | `nodelabsd q license license-type node.license` |
| `LicenseTypes` | All license types (paginated) | `nodelabsd q license license-types` |
| `License` | Single license by ID | `nodelabsd q license license 1` |
| `Licenses` | All licenses across all types (paginated) | `nodelabsd q license licenses` |
| `LicensesByType` | All licenses for a type (paginated) | `nodelabsd q license licenses-by-type node.license` |
| `LicensesByHolder` | Active licenses for a holder (paginated) | `nodelabsd q license licenses-by-holder nodelabs1...` |
| `LicensesByHolderAndType` | Active licenses by holder + type (paginated) | `nodelabsd q license licenses-by-holder-and-type nodelabs1... node.license` |
| `Params` | Module parameters, including the owner | `nodelabsd q license params` |
| `Actions` | The action vocabulary and which actions are unscoped | `nodelabsd q license actions` |
| `Grants` | Every access grant (paginated) | `nodelabsd q license grants` |
| `GrantsByGrantee` | Grants held by an address (paginated) | `nodelabsd q license grants-by-grantee nodelabs1...` |
| `GrantsByScope` | Grants applying to a license type (paginated) | `nodelabsd q license grants-by-scope node.license --action issue` |
| `Can` | Whether a grantee holds an (action, scope) grant | `nodelabsd q license can nodelabs1... issue node.license` |

`GrantsByScope` is a filtered walk — scope is the last key component — so
prefer `GrantsByGrantee` or `Can` on hot paths.

### REST endpoints

All queries are available at `http://localhost:1317/nodelabs/license/...`:

```
GET /nodelabs/license/license_type/{id}
GET /nodelabs/license/license_types
GET /nodelabs/license/license/{id}
GET /nodelabs/license/licenses
GET /nodelabs/license/licenses_by_type/{type_id}
GET /nodelabs/license/licenses_by_holder/{holder}
GET /nodelabs/license/licenses_by_holder/{holder}/{type_id}
GET /nodelabs/license/params
GET /nodelabs/license/actions
GET /nodelabs/license/grants
GET /nodelabs/license/grants/{grantee}
GET /nodelabs/license/grants_by_scope/{scope}
GET /nodelabs/license/can/{grantee}/{action}
```

## EVM precompile

The module ships an EVM precompiled contract that exposes the same transactions
and queries to Solidity contracts. The interface is
[`LicenseI.sol`](precompile/LicenseI.sol); its ABI is embedded from
[`abi.json`](precompile/abi.json).

Default address (`licensetypes.PrecompileAddress`):

```
0x6E6F64656c616273000000000000000000000001
   ^ ascii("nodelabs")                ^ per-precompile slot id
```

The ASCII prefix puts the address far above any plausible upstream
`cosmos/evm` precompile, so silent collision with a future upstream release is
effectively impossible. Operators may register it elsewhere; app wiring panics
at start-up if the EVM keeper's static precompile map already holds this
address.

```go
licensePrecompile := licenseprecompile.NewPrecompile(
    app.LicenseKeeper,
    evmAddrCodec,
    common.HexToAddress(licensetypes.PrecompileAddress),
)
// add to staticPrecompiles before constructing the EVM keeper
```

| Kind | Methods |
|---|---|
| Transactions | `createLicenseType`, `updateLicenseType`, `issueLicenses`, `revokeLicenses` |
| Queries (`view`) | `licenseType`, `licenseTypes`, `license`, `licenses`, `licensesByType`, `licensesByHolder`, `licensesByHolderAndType` |

Calls route through the same msg and query servers as the Cosmos path, so every
authorization rule above applies unchanged — the EVM caller address is
converted to bech32 and becomes the message signer. Ownership and access
grants are **not** exposed through this precompile; manage them with the
this module's own messages.

Solidity events mirror the module's: `LicenseTypeCreated`, `LicenseTypeUpdated`,
`LicenseIssued`, `LicenseRevoked`.

## Consumed keeper surface

Other modules read license state through a narrow surface rather than the
records themselves:

```go
// Bounded count of a holder's active licenses across license types.
// stopAt != 0 stops the walk once the count is decisive; 0 counts everything.
count, err := k.CountActiveLicenses(ctx, holder, []string{"node.license"}, stopAt)

// Existence — the check x/network runs before binding a node type to a type.
found, err := k.HasLicenseType(ctx, "node.license")

// The full record; the grant store's scope validator only needs existence.
lt, found, err := k.GetLicenseType(ctx, "node.license")
```

"Active" means "not revoked": this module never enforces `end_date`, so an
expired-but-unrevoked license still counts. License types meant for counting
should be issued with an empty `end_date` (revocation-only lifecycle).

## Genesis

Example genesis configuration:

```json
{
  "license": {
    "license_types": [
      {
        "id": "node.license",
        "transferrable": true,
        "max_supply": "100",
        "issued_count": "0",
        "active_count": "0",
        "revoked_count": "0"
      }
    ],
    "licenses": [],
    "next_license_id": "1",
    "params": {
      "owner": "nodelabs1owneraddress..."
    },
    "grants": [
      { "grantee": "nodelabs1adminaddress...", "action": "type.create", "scope": "" },
      { "grantee": "nodelabs1adminaddress...", "action": "issue", "scope": "node.license" }
    ]
  }
}
```

Grants and the license types they scope to are in one document, so stateless
validation checks the reference: a grant naming a license type that is not
declared above is rejected before import. On import, license types are written
before grants for the same reason.

License types listed in genesis are written directly and need no `type.create`
grant — that grant gates the `MsgCreateLicenseType` handler, which genesis
import does not go through. The grant is what lets types be added once the
chain is running.

## Events

All state-changing operations emit events:

| Event | Attributes |
|-------|------------|
| `create_license_type` | `license_type_id` |
| `update_license_type` | `license_type_id` |
| `issue_licenses` | `license_type_id`, `holder`, `count` (one event per entry) |
| `revoke_licenses` | `license_type_id`, `holder`, `count` |
| `transfer_ownership` | `module`, `owner` (the new owner) |
| `grant_access` | `module`, `grantee`, `actions` (comma-joined), `scopes` (per-action scope lists, comma-joined within an entry and semicolon-joined between entries) |
| `revoke_access` | `module`, `grantee`, `actions` (comma-joined), `scopes` (comma-joined, positionally paired with `actions`) |

## State Storage

The module uses the `cosmossdk.io/collections` framework for type-safe state management:

| Collection | Key | Value |
|------------|-----|-------|
| `LicenseTypes` | `string` (type ID) | `LicenseType` |
| `Licenses` | `uint64` (license ID) | `License` |
| `NextLicenseID` | (item) | `uint64` (chain-wide next-id sequence, exported in genesis as `next_license_id`) |
| `LicensesByType` | `(string, uint64)` (type ID, license ID) | (keyset, no value; active and revoked) |
| `ActiveLicensesByHolder` | `(string, string, uint64)` (holder, type ID, license ID) | (keyset, no value; active licenses only) |
| `Params` | (item) | `Params` (module owner) |
| `Grants` | `(string, string, string)` (grantee, action, scope) | (keyset, no value) |

The grant key order makes a check a point-read and a grantee's holdings a
prefix walk. The action vocabulary is **not** state: it is compiled into the
binary and served by the `actions` query.

## Module Versioning

The module uses Cosmos SDK's consensus versioning. The current version is `1`. To add a state migration:

1. Bump `ConsensusVersion` in `module.go`
2. Create `keeper/migrator.go` with the migration function
3. Register the migration in `RegisterServices`
4. Add an upgrade handler in the app that calls `RunMigrations`

See the [Cosmos SDK migration docs](https://docs.cosmos.network/main/build/building-modules/upgrade) for details.

## Testing

```bash
go test ./x/license/...
```

Tests cover all message handlers, query handlers, genesis validation, and the
EVM precompile (ABI conformance, transaction and query methods, type
conversion).
