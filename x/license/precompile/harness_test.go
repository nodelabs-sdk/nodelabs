package licenseprecompile

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/tracing"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie/utils"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/core/address"

	sdk "github.com/cosmos/cosmos-sdk/types"
	evmaddress "github.com/cosmos/evm/encoding/address"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
)

// testFixture bundles everything an end-to-end precompile test needs: a real
// licenses keeper backed by an in-memory store,
// the precompile under test, a recording StateDB to capture emitted logs, and
// the EVM-derived owner.
type testFixture struct {
	t          *testing.T
	fixture    *keepertest.LicenseFixture
	keeper     keeper.Keeper
	ctx        sdk.Context
	precompile Precompile
	stateDB    *recordingStateDB
	addrCdc    address.Codec

	OwnerHex  common.Address
	OwnerBech string
}

// newTestFixture wires a fresh precompile against a fresh in-memory keeper,
// installing an EVM-derived address as the license module owner so that
// owner-gated tx methods can be tested using the precompile's
// contract.Caller() path.
func newTestFixture(t *testing.T) *testFixture {
	t.Helper()

	fx := keepertest.NewLicenseFixture(t)
	cdc := evmaddress.NewEvmCodec(sdk.GetConfig().GetBech32AccountAddrPrefix())

	ownerHex := common.HexToAddress("0x1111111111111111111111111111111111111111")
	ownerBech, err := cdc.BytesToString(ownerHex.Bytes())
	require.NoError(t, err)

	// Replace the fixture's random module owner with the EVM-derived one,
	// moving the self-granted type.create along with it so the new owner can
	// create license types.
	require.NoError(t, fx.Keeper.Params.Set(fx.Ctx, licensetypes.Params{Owner: ownerBech}))
	fx.Ungrant(t, fx.Owner, licensetypes.ActionCreateType, "")
	fx.Grant(t, ownerBech, licensetypes.ActionCreateType, "")
	fx.Owner = ownerBech

	p := NewPrecompile(fx.Keeper, cdc, common.HexToAddress(licensetypes.PrecompileAddress))

	return &testFixture{
		t:          t,
		fixture:    fx,
		keeper:     fx.Keeper,
		ctx:        fx.Ctx,
		precompile: *p,
		stateDB:    &recordingStateDB{},
		addrCdc:    cdc,
		OwnerHex:   ownerHex,
		OwnerBech:  ownerBech,
	}
}

// newContract returns a synthetic vm.Contract whose Caller() is the given hex address.
// We never actually execute opcodes against it; only the precompile handlers
// consult Caller() and Input.
func (f *testFixture) newContract(caller common.Address) *vm.Contract {
	return vm.NewContract(caller, f.precompile.Address(), uint256.NewInt(0), 0, nil)
}

// hexFromBech32 is the inverse of the keeper's bech32 addresses for assertions.
func (f *testFixture) hexFromBech32(t *testing.T, bech string) common.Address {
	t.Helper()
	hex, err := bech32ToHex(bech)
	require.NoError(t, err)
	require.NotEqual(t, common.Address{}, hex, "bech32 %q produced zero address", bech)
	return hex
}

// recordingStateDB is a stub vm.StateDB used by the precompile tests. Only AddLog
// is exercised; every other method panics so a regression that calls an
// unexpected StateDB method is flagged loudly instead of silently passing.
type recordingStateDB struct {
	logs []*ethtypes.Log
}

var _ vm.StateDB = (*recordingStateDB)(nil)

func (s *recordingStateDB) AddLog(l *ethtypes.Log) { s.logs = append(s.logs, l) }

// --- everything below this line is interface filler -----------------------

func (s *recordingStateDB) CreateAccount(common.Address)  { panic("unexpected: CreateAccount") }
func (s *recordingStateDB) CreateContract(common.Address) { panic("unexpected: CreateContract") }
func (s *recordingStateDB) SubBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) uint256.Int {
	panic("unexpected: SubBalance")
}
func (s *recordingStateDB) AddBalance(common.Address, *uint256.Int, tracing.BalanceChangeReason) uint256.Int {
	panic("unexpected: AddBalance")
}
func (s *recordingStateDB) GetBalance(common.Address) *uint256.Int { panic("unexpected: GetBalance") }
func (s *recordingStateDB) GetNonce(common.Address) uint64         { panic("unexpected: GetNonce") }
func (s *recordingStateDB) SetNonce(common.Address, uint64, tracing.NonceChangeReason) {
	panic("unexpected: SetNonce")
}
func (s *recordingStateDB) GetCodeHash(common.Address) common.Hash { panic("unexpected: GetCodeHash") }
func (s *recordingStateDB) GetCode(common.Address) []byte          { panic("unexpected: GetCode") }
func (s *recordingStateDB) SetCode(common.Address, []byte) []byte  { panic("unexpected: SetCode") }
func (s *recordingStateDB) GetCodeSize(common.Address) int         { panic("unexpected: GetCodeSize") }
func (s *recordingStateDB) AddRefund(uint64)                       { panic("unexpected: AddRefund") }
func (s *recordingStateDB) SubRefund(uint64)                       { panic("unexpected: SubRefund") }
func (s *recordingStateDB) GetRefund() uint64                      { panic("unexpected: GetRefund") }
func (s *recordingStateDB) GetStateAndCommittedState(common.Address, common.Hash) (common.Hash, common.Hash) {
	panic("unexpected: GetStateAndCommittedState")
}
func (s *recordingStateDB) GetState(common.Address, common.Hash) common.Hash {
	panic("unexpected: GetState")
}
func (s *recordingStateDB) SetState(common.Address, common.Hash, common.Hash) common.Hash {
	panic("unexpected: SetState")
}
func (s *recordingStateDB) GetStorageRoot(common.Address) common.Hash {
	panic("unexpected: GetStorageRoot")
}
func (s *recordingStateDB) IsStorageEmpty(common.Address) bool { panic("unexpected: IsStorageEmpty") }
func (s *recordingStateDB) GetTransientState(common.Address, common.Hash) common.Hash {
	panic("unexpected: GetTransientState")
}
func (s *recordingStateDB) SetTransientState(common.Address, common.Hash, common.Hash) {
	panic("unexpected: SetTransientState")
}
func (s *recordingStateDB) SelfDestruct(common.Address) uint256.Int {
	panic("unexpected: SelfDestruct")
}
func (s *recordingStateDB) HasSelfDestructed(common.Address) bool {
	panic("unexpected: HasSelfDestructed")
}
func (s *recordingStateDB) SelfDestruct6780(common.Address) (uint256.Int, bool) {
	panic("unexpected: SelfDestruct6780")
}
func (s *recordingStateDB) Exist(common.Address) bool { panic("unexpected: Exist") }
func (s *recordingStateDB) Empty(common.Address) bool { panic("unexpected: Empty") }
func (s *recordingStateDB) AddressInAccessList(common.Address) bool {
	panic("unexpected: AddressInAccessList")
}
func (s *recordingStateDB) SlotInAccessList(common.Address, common.Hash) (bool, bool) {
	panic("unexpected: SlotInAccessList")
}
func (s *recordingStateDB) AddAddressToAccessList(common.Address) {
	panic("unexpected: AddAddressToAccessList")
}
func (s *recordingStateDB) AddSlotToAccessList(common.Address, common.Hash) {
	panic("unexpected: AddSlotToAccessList")
}
func (s *recordingStateDB) PointCache() *utils.PointCache { panic("unexpected: PointCache") }
func (s *recordingStateDB) Prepare(params.Rules, common.Address, common.Address, *common.Address, []common.Address, ethtypes.AccessList) {
	panic("unexpected: Prepare")
}
func (s *recordingStateDB) RevertToSnapshot(int)              { panic("unexpected: RevertToSnapshot") }
func (s *recordingStateDB) Snapshot() int                     { panic("unexpected: Snapshot") }
func (s *recordingStateDB) AddPreimage(common.Hash, []byte)   { panic("unexpected: AddPreimage") }
func (s *recordingStateDB) Witness() *stateless.Witness       { panic("unexpected: Witness") }
func (s *recordingStateDB) AccessEvents() *state.AccessEvents { panic("unexpected: AccessEvents") }
func (s *recordingStateDB) Finalise(bool)                     { panic("unexpected: Finalise") }
