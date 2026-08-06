# Module Parameters

Reference for the on-chain parameters of the `x/license` and `x/network` modules.

Both parameter sets are updated through `MsgUpdateParams` (governance authority) and read
through the `Params` gRPC query. Both default sets are deliberately chain-neutral — a
consuming chain seeds its real values in genesis or an upgrade handler.

---

## `x/license`

- Proto: [`proto/license/v1/license.proto`](../proto/license/v1/license.proto) (`message Params`)
- Defaults & validation: [`x/license/types/params.go`](../x/license/types/params.go)

| # | Field | Type | Default | Validation |
|---|-------|------|---------|------------|
| 1 | `owner` | `string` (bech32 address) | `""` | Empty is valid and means "not yet set"; a non-empty value must be a valid bech32 address |

**`owner`** — the address allowed to grant and revoke access within the module. It is empty
until governance sets it, and while empty no grant can be made, so the module fails closed.

Owning the module confers the right to *delegate* actions, not the actions themselves: the
owner must grant itself `type.create` like anyone else.

---

## `x/network`

- Proto: [`proto/network/v1/network.proto`](../proto/network/v1/network.proto) (`message Params`)
- Defaults & validation: [`x/network/types/params.go`](../x/network/types/params.go)

| # | Field | Type | Default | Validation |
|---|-------|------|---------|------------|
| 1 | `activation_limit_multiplier` | `uint64` | `3` | must be > 0 |
| 2 | `spam_limit_multiplier` | `uint64` | `9` | must be ≥ `activation_limit_multiplier` |
| 3 | `max_activation_keys` | `uint64` | `5` | must be > 0 |
| 4 | `recent_key_limit` | `uint64` | `10` | must be > 0 |
| 5 | `recent_key_window` | `google.protobuf.Duration` (stdduration, non-nullable) | `24h` | must be > 0 |
| 6 | `status_daily_limit` | `uint64` | `10` | must be > 0 |
| 7 | `deauthorize_fee` | `repeated cosmos.base.v1beta1.Coin` (cast to `sdk.Coins`) | `sdk.NewCoins()` (empty — charge disabled) | must pass `Coins.Validate()` |
| 8 | `max_gasless_gas` | `uint64` | `300_000` | must be > 0 |
| 9 | `max_gasless_msgs` | `uint64` | `10` | must be > 0 |
| 10 | `max_gasless_tx_bytes` | `uint64` | `50_000` | must be > 0 |
| 11 | `owner` | `string` (bech32 address) | `""` | Empty is valid and means "not yet set"; a non-empty value must be a valid bech32 address |

### Grouped by what they govern

#### Activation limits

- **`activation_limit_multiplier`** — scales a node type's active license count into that
  node type's activation limit.
- **`spam_limit_multiplier`** — scales a node type's active license count into the
  recent-activity ceiling that hard-stops activation bursts. Validation enforces that it is
  never below `activation_limit_multiplier`.

#### Activation keys

- **`max_activation_keys`** — maximum number of active activation keys per operator.
- **`recent_key_limit`** — maximum number of activation keys an operator may create within
  `recent_key_window`.
- **`recent_key_window`** — the sliding window `recent_key_limit` is measured over.

#### Node status

- **`status_daily_limit`** — maximum number of `MsgUpdateNodeStatus` per node per UTC day.

#### Fees

- **`deauthorize_fee`** — charged by the `MsgDeauthorizeActivationKey` handler as a bank
  transfer to the fee collector, on top of regular gas. Empty disables the charge; chains
  that want it seed a denominated value. The default is empty because the module cannot name
  an arbitrary chain's denom.

#### Gasless transaction envelope

- **`max_gasless_gas`** — caps the declared gas wanted of a gasless transaction.
- **`max_gasless_msgs`** — caps the number of messages in a gasless transaction.
- **`max_gasless_tx_bytes`** — caps the encoded size of a gasless transaction.

#### Access control

- **`owner`** — same semantics as the license module's `owner`: the address allowed to grant
  and revoke access within the module, empty until governance sets it, failing closed while
  empty. The owner must grant itself `nodetype.create` like anyone else.

---

## Fail-closed notes

- Both modules' `owner` defaults to empty, so no grant can be made until governance sets one.
- `x/network` activation still fail-closes out of the box, but that comes from the node type
  registry rather than a param: with no node types registered, no node type resolves and
  nothing can activate.
