# Network Module

The `x/network` module manages the node fleet: operator wallets authorize
activation keys, those keys activate nodes, and nodes report periodic status
heartbeats. How many nodes an operator may run is derived from the licenses it
holds in the [`x/license`](../license/README.md) module, per node type.

## Overview

- **Node types** are registered records that bind a node class (e.g.
  `nodelabs.trust`) to exactly one existing license type. The binding is
  one-to-one and fixed at creation, and records are never removed
- **Operators** are wallets that hold licenses. An operator is registered
  implicitly by authorizing an activation key or activating a node
- **Activation keys** are addresses an operator authorizes to activate nodes on
  its behalf. An address is permanently bound to the first operator that
  authorizes it, even after deauthorization
- **Nodes** are activated by an activation key under an operator, carry a node
  type, and are never deleted — deactivation is irreversible and the record is
  what keeps the address burned
- **Entitlement is per node type.** A node type's activation limits come from
  the operator's active licenses of the license type that node type is bound
  to; licenses of any other type do not count
- **Four messages are gasless**, admitted at zero fee subject to ante-handler
  standing checks and rolling daily quotas
- **Access grants** for the administrative messages live in this module: the
  owner is the `owner` parameter, and grants are module-wide. The grant
  machinery is the shared [`x/access`](../access) library

## Installation

### As a dependency in another Cosmos SDK chain

```bash
go get github.com/nodelabs-sdk/nodelabs
```

### Wiring into app.go (manual)

```go
import (
    network "github.com/nodelabs-sdk/nodelabs/x/network"
    networkante "github.com/nodelabs-sdk/nodelabs/x/network/ante"
    networkkeeper "github.com/nodelabs-sdk/nodelabs/x/network/keeper"
    networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)
```

1. Add the store key:

```go
keys := storetypes.NewKVStoreKeys(
    // ... existing keys
    networktypes.StoreKey,
)
```

2. Create the keeper. It consumes the license, account, and bank keepers, so it
   is constructed after all three:

```go
app.NetworkKeeper = networkkeeper.NewKeeper(
    appCodec,
    runtime.NewKVStoreService(keys[networktypes.StoreKey]),
    logger,
    authAddr, // governance authority address
    app.LicenseKeeper,
    app.AccountKeeper,
    app.BankKeeper,
)
```

3. Register the module:

```go
app.ModuleManager = module.NewManager(
    // ... existing modules
    network.NewAppModule(appCodec, app.NetworkKeeper),
)
```

4. Add to genesis ordering, **after** `license` — the network module consumes
   the license keeper:

```go
genesisModuleOrder := []string{
    // ... existing modules
    licensetypes.ModuleName,
    networktypes.ModuleName,
}
```

5. Compose the gasless ante machinery (see [Gasless transactions](#gasless-transactions)).

### Wiring via depinject

The module supports dependency injection. Add the module proto config to your
app config and import the package:

```go
import _ "github.com/nodelabs-sdk/nodelabs/x/network"
```

The `init()` function in `depinject.go` registers the module. `ProvideModule`
resolves the codec, store service, and the license, account, and bank keepers
from the DI container.

## Concepts

### Node types

A node type is the registry entry that makes a node class activatable:

| Field | Description |
|-------|-------------|
| `id` | Registrant-supplied identifier (e.g. `nodelabs.trust`) |
| `license_type_id` | The license type backing this node type |

Registration is gated on the module-wide `nodetype.create` grant, which is the
whole authorization: holding it authorizes defining node types against any
existing license type. The registering address is not recorded, because it
would confer nothing after the fact.

The license type must exist. Binding to an absent one would mint a node type
that can never activate anything — the activation limit counts licenses of that
type, so the count would be permanently zero — and node types cannot be removed
or re-pointed to undo it.

Because bindings are one-to-one and permanent, `nodetype.create` is a strong
grant: a node type registered against a license type occupies it for good.
Grant it only to addresses trusted with the registry as a whole.

The binding is one-to-one in both directions — a node type names exactly one
license type, and a license type backs at most one node type. Ids are
single-use, the binding is fixed at creation, and records are never removed: a
node's type must stay resolvable for the life of the chain.

**The registry is the activation allowlist and it fails closed.** With no node
types registered, nothing can activate.

### Operators and activation keys

An operator is a wallet that holds licenses. It does not sign activations
itself: it authorizes activation keys, and those keys sign `MsgActivateNode`.
Separating the two means a hosting provider can run a fleet without ever
holding the operator's key.

| Field | Description |
|-------|-------------|
| `address` | The activation key's account address |
| `operator` | The wallet that authorized it |
| `created_at` | Block time of authorization, used for the recent-creation window |
| `status` | `active` or `disabled` |

An activation address is **globally unique and permanently bound** to the first
operator that authorizes it. Deauthorization flips the status but keeps the
record, so the address can never be re-authorized — by that operator or any
other — and it keeps counting toward the recent-creation window.

Authorization is bounded two ways: `max_activation_keys` caps how many keys an
operator may have *active* at once, and `recent_key_limit` caps how many it may
create within `recent_key_window` (disabled keys included).

### Nodes

| Field | Description |
|-------|-------------|
| `address` | The node's account address |
| `operator` | The wallet whose licenses back this node |
| `activated_by` | The activation key that activated it |
| `type` | The node type id; must name a registered node type |
| `status` | `active` or `deactivated` |
| `last_active_time` | Block time of the most recent counted activity |

Node addresses are single-use. Records are never removed, so a deactivated
node's record is exactly what prevents its address from being re-activated.
Deactivation is irreversible and deliberately does not touch `last_active_time`
or the recent-activity index — deactivated nodes keep counting in the recent
window, so churning a fleet cannot buy fresh allowance.

The node does not sign its own activation, so delayed activation works by
construction: a key can activate a node address before that node ever comes
online.

### Address canonicalization

Every address field is validated in its **canonical bech32 encoding**, not
merely as decodable bech32. BIP-173 permits an all-uppercase encoding of the
same payload and the bech32 library normalizes rather than rejects it, so
`TSC1ABC…` and `tsc1abc…` decode to identical bytes and both authenticate as
the same account. This module keys state on the address *string*, and it is the
presence of a record that burns a node address or an activation address —
accepting a non-canonical alias would let a caller re-activate a deactivated
node simply by changing the case.

### Activation limits

`MsgActivateNode` runs a fixed algorithm. Every quantity in it is scoped to the
node type being activated — the tallies, the activity window, and the license
count alike:

0. The signing activation key must exist, be active, and be bound to the named
   operator
1. The node address must never have been activated
2. `count` = the operator's active licenses of the node type's license type.
   Zero fails with a distinct "no active licenses" error.
   `limit = activation_limit_multiplier × count`
3. If the operator's all-time node total for this type is below `limit`, allow
4. Else if the recent-active count (today + yesterday, UTC) is below `limit`, allow
5. Else if the recent-active count has reached `spam_limit_multiplier × count`,
   fail "recent activation limit exceeded"
6. Else the currently-active count decides against `limit`

The license walk is bounded by the decision being made rather than by the
operator's holdings: it stops as soon as the count can no longer change the
outcome.

Query [`NodeCounts`](#queries) to pre-check an activation without submitting one.

### Activity and day buckets

Counted activity — activation and status updates — moves a node's single entry
in the recent-activity index into the current UTC day bucket and stamps
`last_active_time`. The index holds exactly one entry per node at all times, so
the "today + yesterday" count is two prefix walks with no dedup.

Daily counters (`ActivityCounter`) reset when the UTC day of the block time
differs from the UTC day of the counter's `latest_time`. There is no scheduled
reset; the roll happens on the next counted action.

### Licensing gates

Two gates guard operator standing, and the narrower one is used wherever a node
type is in hand:

- `EnsureOperatorLicensedForNodeType` — the operator must hold at least one
  active license of the type backing *that* node type. Used by `ActivateNode`
  admission and by the daily `UpdateNodeStatus` roll (using the node record's
  own type)
- `EnsureOperatorLicensed` — the operator must hold at least one active license
  of *any* license type backing a registered node type. Used only by
  `AuthorizeActivationKey`, which happens before any node exists and so has
  nothing narrower to check

"Active" means "not revoked": `x/license` never enforces `end_date`, so an
expired-but-unrevoked license still counts. License types meant for counting
should be issued with an empty `end_date`.

## Gasless transactions

`MsgAuthorizeActivationKey`, `MsgActivateNode`, `MsgDeactivateNode`, and
`MsgUpdateNodeStatus` are admitted at zero fee. A tx is gasless only if **every**
msg in it is allowlisted **and** the declared fee is exactly zero, which
prevents smuggling paid msgs into free txs and vice versa.

The `x/network/ante` package ships the pieces; the app composes them into its
cosmos ante chain (the EVM mono-handler path is untouched — gasless msgs never
arrive as Ethereum txs):

```go
gaslessAllowlist := networkante.NewAllowlist(networktypes.GaslessMessages())
admissionRouter := networkante.NewAdmissionRouter(app.NetworkKeeper, networktypes.GaslessMessages())
```

| Piece | Role | Placement |
|---|---|---|
| `NewGaslessCapsDecorator` | Hard caps on declared gas, msg count, encoded size | Before `SetUpContextDecorator` — with the fee forced to zero, nothing else bounds declared gas |
| `NewSkipForGaslessDecorator` | Runs the wrapped decorator only for paid txs | Wraps `MinGasPriceDecorator`, which would reject a zero fee |
| `NewGaslessTxFeeChecker` | Clears gasless txs at zero fee and zero priority, delegates the rest | Wraps the dynamic fee checker passed to `NewDeductFeeDecorator` |
| `NewGaslessAdmissionDecorator` | Per-msg standing checks + daily quota | After signature verification, so checks run against authenticated signers |

`NewAllowlist` and `AdmissionRouter.Merge` both take multiple lists, so a
consuming chain can add its own gasless msgs (e.g. attestation msgs) alongside
these.

Because admission runs in the ante chain, **its counter writes are committed
even when the msg handler later fails** — failing txs still burn quota — and
over-limit spam is rejected at CheckTx before reaching a block. Handlers re-read
the counters and never re-increment.

| Msg | Standing check | Counter keyed on | Daily limit |
|---|---|---|---|
| `MsgAuthorizeActivationKey` | Operator holds ≥ 1 active counted license | Operator | `recent_key_limit` |
| `MsgActivateNode` | Key is active and bound to the operator; operator licensed for the node type | Activation key **+ node type** | `spam_limit_multiplier` × licenses of that node type's license type |
| `MsgDeactivateNode` | Signer is the operator, the node's original still-active key, or the node itself; node is active | Signer | `status_daily_limit` |
| `MsgUpdateNodeStatus` | Node is active; on the first counted tx of each UTC day, the operator must still be licensed for the node's type | Node | `status_daily_limit` |

The activate counter carries a node-type dimension because the ceiling it is
measured against does. An operator gets one daily allowance per node type,
sized by the licenses backing that type; a counter pooled across types would be
compared against whichever type the current message names, so working through a
well-licensed type's generous allowance would exhaust a sparsely-licensed one's
quota before a single node of it was activated. The other kinds are measured
against flat params and so have nothing to scope by.

Deactivation is deliberately **not** license-gated: winding down a fleet must
stay possible after revocation.

## Access

This module does not scope its actions: every grant is module-wide and carries
the empty scope.

| Action | Authorizes |
|---|---|
| `wallet.create` | `MsgCreateOperatorAccount` |
| `nodetype.create` | `MsgCreateNodeType`, against any existing license type |

```bash
nodelabsd tx network grant-access nodelabs1admin... wallet.create,nodetype.create --from owner
nodelabsd tx network revoke-access nodelabs1admin... wallet.create --from owner
nodelabsd query network grants-by-grantee nodelabs1admin...
nodelabsd query network actions
```

Because grants here are always module-wide, the commands take no scope
argument, and a message that carries one is **rejected** rather than ignored:
the shared grant store would happily file it under that scope, where every
check in this module — all of which look under the empty scope — would miss it.

### Module owner

The module owner is the `owner` parameter. It is the only address that can
grant and revoke access or hand the module over. Ownership confers no other
rights: the owner must grant itself `nodetype.create` like anyone else.

The owner is empty until it is set, and while empty the module fails closed —
every owner-gated message returns `ErrOwnerNotSet`. It is set in genesis or by
governance (`MsgUpdateParams`, which is also the recovery path for a lost or
compromised key), and can be handed off directly by the current owner
(`MsgTransferOwnership`).

`MsgTransferOwnership` is the one message that lets a non-authority signer
write a parameter. It reads the current parameter set back and replaces only
`owner`, so a handoff can never reset an activation limit or a gasless cap.

## Messages

### MsgCreateNodeType
Register a node type bound to a license type. Signer must hold
`nodetype.create`, and the named license type must exist. The signing address
is not stored on the record. Regular gas tx.

```bash
nodelabsd tx network create-node-type nodelabs.trust nodelabs.node --from admin
```

### MsgCreateOperatorAccount
Create an on-chain account for an operator wallet. Signer must hold
`wallet.create`. Fails if the account already exists. Regular gas tx.

This is a fallback: `x/license` already creates a holder's account on issuance,
so it is only needed for wallets that predate that behaviour or hold no
licenses yet.

```bash
nodelabsd tx network create-operator-account nodelabs1wallet... --from admin
```

### MsgAuthorizeActivationKey
Authorize a new activation key for the signing operator. The address must never
have been authorized by anyone, and must differ from the operator wallet.
Creates an account for the activation address. **Gasless.**

```bash
nodelabsd tx network authorize-activation-key nodelabs1key... --from operator
```

### MsgDeauthorizeActivationKey
Disable an activation key. Signer must be the operator the key is bound to. The
record is retained, so the address stays burned and keeps counting toward the
recent-creation window. Regular gas tx, **plus** the `deauthorize_fee` param
charged as a bank transfer from the operator to the fee collector — a
fee-checker-immune cost, since an ante check against the declared fee is
bypassable under dynamic-fee semantics.

```bash
nodelabsd tx network deauthorize-activation-key nodelabs1key... --from operator
```

### MsgActivateNode
Activate a node under an operator, signed by an authorized activation key. Runs
the [activation-limit algorithm](#activation-limits) and creates an account for
the node address. **Gasless.**

```bash
nodelabsd tx network activate-node nodelabs1op... nodelabs1node... nodelabs.trust --from activation-key
```

### MsgDeactivateNode
Deactivate a node, irreversibly. Valid signers are the operator, the node's
original activation key *while that key is still active* (so deauthorizing a
provider fully revokes its power over already-activated nodes), or the node
itself. **Gasless.**

```bash
nodelabsd tx network deactivate-node nodelabs1node... --from operator
```

### MsgUpdateNodeStatus
Record a node's periodic status heartbeat. Signer is the node, which must be
active. The payload is emitted as a `node_status` event and **not stored in
state**; the chain treats every field as opaque, checking only sizes
(512 bytes per field, 100 workload entries). **Gasless**, limited to
`status_daily_limit` per node per UTC day.

Not exposed via CLI — the nested payload doesn't map to positional args, and
node software submits it programmatically.

### MsgUpdateParams
Update the module parameters. Signer must be the module authority (`x/gov` by
default). All parameters must be supplied. Submitted via governance proposal.

## Queries

All queries are available via gRPC, REST, and CLI (auto-generated via autocli).

| Query | Description | CLI |
|-------|-------------|-----|
| `Params` | Module parameters, including the owner | `nodelabsd q network params` |
| `Actions` | The action vocabulary | `nodelabsd q network actions` |
| `Grants` | Every access grant (paginated) | `nodelabsd q network grants` |
| `GrantsByGrantee` | Grants held by an address (paginated) | `nodelabsd q network grants-by-grantee nodelabs1...` |
| `Can` | Whether a grantee holds an action grant | `nodelabsd q network can nodelabs1... wallet.create` |

There is no by-scope grant query, unlike [`x/license`](../license/README.md):
this module does not scope its actions, so every grant sits under the same
empty scope and such a query would just be `Grants` under another name.
| `Node` | Single node by address | `nodelabsd q network node nodelabs1node...` |
| `Nodes` | All nodes (paginated), optional status filter | `nodelabsd q network nodes --status active` |
| `NodesByOperator` | An operator's nodes (paginated), optional status filter | `nodelabsd q network nodes-by-operator nodelabs1op...` |
| `NodeType` | Single node type by id | `nodelabsd q network node-type nodelabs.trust` |
| `NodeTypes` | Registered node types (paginated), optional license-type filter | `nodelabsd q network node-types --license-type-id nodelabs.node` |
| `ActivationKey` | Single activation key by address | `nodelabsd q network activation-key nodelabs1key...` |
| `ActivationKeys` | An operator's keys, active and disabled (paginated) | `nodelabsd q network activation-keys nodelabs1op...` |
| `NodeCounts` | Per-node-type tallies and limits for an operator | `nodelabsd q network node-counts nodelabs1op...` |

Node records are retained after deactivation, so an unfiltered `Nodes` listing
returns every node ever activated. Leave `--status` unset for all statuses.

`NodeCounts` returns one entry per registered node type (or just the one named
by `--node-type`), each carrying `total`, `active`, `recent_active`,
`license_count`, `activation_limit`, and `spam_limit`. It is the pre-check a
hosting provider runs before attempting an activation.

### REST endpoints

All queries are available at `http://localhost:1317/nodelabs/network/...`:

```
GET /nodelabs/network/params
GET /nodelabs/network/node/{address}
GET /nodelabs/network/nodes
GET /nodelabs/network/nodes_by_operator/{operator}
GET /nodelabs/network/node_type/{id}
GET /nodelabs/network/node_types
GET /nodelabs/network/activation_key/{address}
GET /nodelabs/network/activation_keys/{operator}
GET /nodelabs/network/node_counts/{operator}
```

## Parameters

| Param | Default | Description |
|-------|---------|-------------|
| `activation_limit_multiplier` | `3` | Scales a node type's active license count into that node type's activation limit |
| `spam_limit_multiplier` | `9` | Scales it into the recent-activity ceiling that hard-stops activation bursts. Must not be below `activation_limit_multiplier` |
| `max_activation_keys` | `5` | Maximum *active* activation keys per operator |
| `recent_key_limit` | `10` | Maximum keys an operator may create within `recent_key_window`; also the daily gasless authorize quota |
| `recent_key_window` | `24h` | Sliding window for `recent_key_limit` |
| `status_daily_limit` | `10` | Maximum `MsgUpdateNodeStatus` per node per UTC day; also the daily gasless deactivate quota |
| `deauthorize_fee` | *(empty)* | Charged by the `MsgDeauthorizeActivationKey` handler on top of gas. Empty disables the charge |
| `max_gasless_gas` | `300000` | Cap on a gasless tx's declared gas |
| `max_gasless_msgs` | `10` | Cap on messages in a gasless tx |
| `max_gasless_tx_bytes` | `50000` | Cap on a gasless tx's encoded size |

Defaults are chain-neutral: `deauthorize_fee` is empty because this module
cannot name a chain's denom. Chains seed their real values in genesis or an
upgrade handler. Activation still fails closed out of the box — that comes from
the empty node type registry rather than a param.

## Genesis

```json
{
  "network": {
    "params": {
      "activation_limit_multiplier": "3",
      "spam_limit_multiplier": "9",
      "max_activation_keys": "5",
      "recent_key_limit": "10",
      "recent_key_window": "86400s",
      "status_daily_limit": "10",
      "deauthorize_fee": [],
      "max_gasless_gas": "300000",
      "max_gasless_msgs": "10",
      "max_gasless_tx_bytes": "50000",
      "owner": "nodelabs1owneraddress..."
    },
    "node_types": [
      {
        "id": "nodelabs.trust",
        "license_type_id": "nodelabs.node"
      }
    ],
    "nodes": [],
    "activation_keys": [],
    "node_status_counters": [],
    "gasless_counters": [],
    "grants": [
      { "grantee": "nodelabs1adminaddress...", "action": "wallet.create", "scope": "" },
      { "grantee": "nodelabs1adminaddress...", "action": "nodetype.create", "scope": "" }
    ]
  }
}
```

Every grant must carry an empty scope: this module does not scope its actions,
and stateless genesis validation rejects a non-empty one.

Only primary records are exported. The denormalized structures — the operator
set, the operator indexes, the per-(operator, node type) tallies, the
recent-activity index, and the node-type-by-license-type map — are **derived on
import**, so they can never disagree with the records they mirror.

`gasless_counters` is exported so an export/import does not hand every signer a
fresh day's quota.

Validation enforces the invariants the msg handlers maintain unconditionally:
canonical addresses, no duplicates, every node's `type` present in
`node_types`, every node's `activated_by` a listed key bound to the same
operator, the one-to-one license-type binding, and set timestamps.
Param-dependent limits (e.g. `max_activation_keys`) are deliberately **not**
enforced — governance may lower a param after state accrued under a higher one,
and export/import must round-trip.

`license_type_id` is checked for shape only. Whether it names a real license
type is a fact about `x/license` state and is invisible from here; the msg
handler enforces it at registration time.

## Events

| Event | Attributes |
|-------|------------|
| `create_operator_account` | `wallet`, `signer` |
| `create_node_type` | `node_type`, `signer`, `license_type_id` |
| `authorize_activation_key` | `operator`, `activation_address` |
| `deauthorize_activation_key` | `operator`, `activation_address`, `fee` |
| `activate_node` | `operator`, `activation_address`, `node_address`, `node_type` |
| `deactivate_node` | `operator`, `node_address`, `signer` |
| `node_status` | `node_address`, `operator`, `node_type`, plus the payload fields `device`, `os`, `hostname`, `node_info`, `workloads`, `memory`, `cpu`, `storage` |

`node_status` is the only record of a heartbeat's payload — indexers read it
from the event stream, since nothing is written to state.

## State Storage

The module uses the `cosmossdk.io/collections` framework for type-safe state
management.

Primary records:

| Collection | Key | Value |
|------------|-----|-------|
| `Params` | (item) | `Params` |
| `Nodes` | `string` (node address) | `Node` (never removed) |
| `ActivationKeys` | `string` (activation address) | `ActivationKey` (never removed) |
| `NodeTypes` | `string` (node type id) | `NodeType` (never removed) |
| `NodeStatusCounters` | `string` (node address) | `ActivityCounter` |

Derived structures, rebuilt on genesis import:

| Collection | Key | Value |
|------------|-----|-------|
| `Operators` | `string` (operator) | (keyset) |
| `OperatorNodes` | `(string, string)` (operator, node address) | (keyset; active and deactivated) |
| `OperatorActivationKeys` | `(string, string)` (operator, activation address) | (keyset; active and disabled) |
| `OperatorNodeCounts` | `(string, string)` (operator, node type) | `OperatorNodeCounts{total, active}` |
| `RecentNodeActivity` | `(string, string, uint64, string)` (operator, node type, day epoch, node address) | (keyset; exactly one entry per node) |
| `GaslessCounters` | `(string, string, string)` (kind, signer, scope) | `ActivityCounter` |
| `NodeTypeByLicenseType` | `string` (license type id) | `string` (node type id) |

The node-type dimension on `OperatorNodeCounts` and `RecentNodeActivity` is
load-bearing: activation limits are per node type, so a tally pooled across
types would let one type's nodes consume another's allowance.

`NodeTypeByLicenseType` is a map rather than an index because the binding is
one-to-one — the map shape is what makes a second binding to the same license
type impossible to represent.

`GaslessCounters` kinds are `authorize`, `activate`, and `deactivate`;
`MsgUpdateNodeStatus` uses the separate `NodeStatusCounters` store. Only the
`activate` kind uses the scope component, which holds the node type; the others
take the empty scope, so each kind has exactly one key form and a counter
lookup stays a point-read.

Access grants are stored in this module under the `0x07` prefix, keyed
`(grantee, action, scope)`. Scope is always empty here. The action vocabulary
is **not** state: it is compiled into the binary and served by the `actions`
query.

## Invariants

Both derived structures are maintained incrementally by the handlers, so a path
that updates a node without updating its tally leaves state that is internally
inconsistent but individually well-formed. Two invariants recompute them from
the `Nodes` records — the same derivation `InitGenesis` performs — and report
any disagreement:

| Method | Checks |
|---|---|
| `CheckOperatorNodeCounts` | The stored `{total, active}` tally matches the node records, per `(operator, node type)` |
| `CheckRecentNodeActivity` | The index holds exactly one entry per node, in the day bucket of that node's `last_active_time` |
| `CheckInvariants` | Both of the above, reporting all problems rather than the first |

```go
if err := app.NetworkKeeper.CheckInvariants(ctx); err != nil {
    // state has drifted from the node records
}
```

`CheckOperatorNodeCounts` exists specifically because `DeactivateNode`
decrements `active` saturatingly (`if counts.Active > 0`). That guard is
deliberate — `active` is a `uint64`, and wrapping around would hand out an
effectively unlimited activation allowance — but it means a drift bug fails
quietly rather than loudly. The check is what makes it loud.

These are plain keeper methods returning `error`, **not** `sdk.Invariant`
registered on an invariant registry. That machinery is deprecated along with
`x/crisis` as of Cosmos SDK v0.53 and is removed in the next release, so a
check written against it would stop compiling on the next dependency bump.
`AppModule.RegisterInvariants` is therefore a deliberate no-op. Call these from
tests, an upgrade handler, or a debug query instead.

## Consumed keeper surface

The keeper exposes a small read surface for attestation-style modules built on
top of the fleet:

```go
node, active, err := k.IsActiveNode(ctx, nodeAddr)
node, found, err := k.GetNode(ctx, addr)
key, found, err := k.GetActivationKey(ctx, addr)
counts, err := k.GetOperatorNodeCounts(ctx, operator, nodeType)
count, err := k.CountRecentActiveNodes(ctx, operator, nodeType, blockTime)
```

`TouchNodeActivity(ctx, nodeAddr)` is the write half: it records counted
activity for a node, moving its recent-activity entry into the current day
bucket. Callers are responsible for checking the node is active first.

The module in turn consumes narrow interfaces declared in
`types/expected_keepers.go`: `LicenseKeeper` (`CountActiveLicenses`,
`HasLicenseType`), `AccountKeeper`, and `BankKeeper`.

## Module Versioning

The module uses Cosmos SDK's consensus versioning. The current version is `1`.
To add a state migration:

1. Bump `ConsensusVersion` in `module.go`
2. Create `keeper/migrator.go` with the migration function
3. Register the migration in `RegisterServices`
4. Add an upgrade handler in the app that calls `RunMigrations`

See the [Cosmos SDK migration docs](https://docs.cosmos.network/main/build/building-modules/upgrade) for details.

## Testing

```bash
go test ./x/network/...
```

Tests cover all message handlers, the activation-limit algorithm, ante
admission and quota rolling, query handlers, and genesis round-tripping.
