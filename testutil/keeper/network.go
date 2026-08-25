package keeper

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/collections"
	"cosmossdk.io/log/v2"
	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/store/v2"
	storetypes "github.com/cosmos/cosmos-sdk/store/v2/types"
	dbm "github.com/cosmos/cosmos-db"

	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	licensekeeper "github.com/nodelabs-sdk/nodelabs/x/license/keeper"
	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
	networkkeeper "github.com/nodelabs-sdk/nodelabs/x/network/keeper"
	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// FakeAccountKeeper is a map-backed types.AccountKeeper for keeper tests.
type FakeAccountKeeper struct {
	accounts map[string]sdk.AccountI
	nextNum  uint64
}

func NewFakeAccountKeeper() *FakeAccountKeeper {
	return &FakeAccountKeeper{accounts: make(map[string]sdk.AccountI)}
}

func (f *FakeAccountKeeper) GetAccount(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	return f.accounts[addr.String()]
}

func (f *FakeAccountKeeper) HasAccount(_ context.Context, addr sdk.AccAddress) bool {
	_, ok := f.accounts[addr.String()]
	return ok
}

func (f *FakeAccountKeeper) NewAccountWithAddress(_ context.Context, addr sdk.AccAddress) sdk.AccountI {
	acc := authtypes.NewBaseAccountWithAddress(addr)
	if err := acc.SetAccountNumber(f.nextNum); err != nil {
		panic(err)
	}
	f.nextNum++
	return acc
}

func (f *FakeAccountKeeper) SetAccount(_ context.Context, acc sdk.AccountI) {
	f.accounts[acc.GetAddress().String()] = acc
}

// FakeBankKeeper records module transfers for keeper tests.
type FakeBankKeeper struct {
	// Transfers records each SendCoinsFromAccountToModule call.
	Transfers []FakeTransfer
	// FailWith, when set, makes transfers fail (e.g. to simulate an operator
	// that cannot pay the deauthorize fee).
	FailWith error
}

// FakeTransfer is one recorded SendCoinsFromAccountToModule call.
type FakeTransfer struct {
	From   string
	Module string
	Amount sdk.Coins
}

func (f *FakeBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	if f.FailWith != nil {
		return f.FailWith
	}
	f.Transfers = append(f.Transfers, FakeTransfer{From: senderAddr.String(), Module: recipientModule, Amount: amt})
	return nil
}

// NetworkFixture bundles a network keeper with the license keeper it consumes,
// sharing one CommitMultiStore, plus fake account/bank keepers. Owner owns both
// modules, and network params are otherwise the module defaults — what an
// operator may activate comes from the node type registry, not from params.
//
// Two node type / license type pairs are registered, trust and nano, so that
// per-node-type entitlement is exercised by default.
type NetworkFixture struct {
	Keeper        networkkeeper.Keeper
	LicenseKeeper licensekeeper.Keeper
	AccountKeeper *FakeAccountKeeper
	BankKeeper    *FakeBankKeeper
	Ctx           sdk.Context
	Authority     string
	Owner         string

	// NodeType and LicenseType are the fixture's default pair, node type
	// nodelabs.trust backed by license type nodelabs.node.trust. The shared
	// activateNode helper uses NodeType, and IssueLicenses issues LicenseType.
	NodeType    string
	LicenseType string

	// NanoNodeType and NanoLicenseType are a second registered pair,
	// nodelabs.nano backed by nodelabs.node.nano. Entitlement is per node
	// type, and the fixture issues no nano licences, so this is the pair to
	// reach for when asserting that one type's allowance does not leak into
	// another's. Use IssueLicensesOfType to licence it.
	NanoNodeType    string
	NanoLicenseType string
}

// NetworkFixtureBlockTime is the fixture context's initial block time; tests
// that roll days advance from here via WithBlockTime.
var NetworkFixtureBlockTime = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

// NewNetworkFixture returns a network keeper wired to license and fake
// account/bank keepers for testing. Each module's owner is its own parameter
// and each module's grants live in its own state.
func NewNetworkFixture(t testing.TB) *NetworkFixture {
	t.Helper()

	licenseStoreKey := storetypes.NewKVStoreKey(licensetypes.StoreKey)
	networkStoreKey := storetypes.NewKVStoreKey(networktypes.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger())
	stateStore.MountStoreWithDB(licenseStoreKey, storetypes.StoreTypeIAVL, db)
	stateStore.MountStoreWithDB(networkStoreKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	registry := codectypes.NewInterfaceRegistry()
	licensetypes.RegisterInterfaces(registry)
	networktypes.RegisterInterfaces(registry)
	cdc := codec.NewProtoCodec(registry)

	owner := sample.AccAddress()
	authority := sample.AccAddress()

	ak := NewFakeAccountKeeper()
	bk := &FakeBankKeeper{}

	lk := licensekeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(licenseStoreKey),
		log.NewNopLogger(),
		authority,
		ak,
	)

	nk := networkkeeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(networkStoreKey),
		log.NewNopLogger(),
		authority,
		lk,
		ak,
		bk,
	)

	ctx := sdk.NewContext(stateStore, tmproto.Header{Height: 1, Time: NetworkFixtureBlockTime}, false, log.NewNopLogger())

	require.NoError(t, lk.Params.Set(ctx, licensetypes.Params{Owner: owner}))

	const (
		trustNodeType    = "nodelabs.trust"
		trustLicenseType = "nodelabs.node.trust"
		nanoNodeType     = "nodelabs.nano"
		nanoLicenseType  = "nodelabs.node.nano"
	)

	networkParams := networktypes.DefaultParams()
	networkParams.Owner = owner
	require.NoError(t, nk.Params.Set(ctx, networkParams))

	// Two node types, each on its own license type, because the binding is
	// one-to-one and entitlement is per node type. Registering both by default
	// means every test runs against a chain where a second, unlicensed node
	// type exists — so a limit that leaked across types would show up broadly
	// rather than only in the tests written to look for it.
	//
	// License types are created up front rather than lazily by the first
	// IssueLicenses call: the node types are bound to them, and a binding is
	// only legal against a license type that already exists.
	for _, pair := range []struct{ nodeType, licenseType string }{
		{trustNodeType, trustLicenseType},
		{nanoNodeType, nanoLicenseType},
	} {
		require.NoError(t, lk.LicenseTypes.Set(ctx, pair.licenseType, licensetypes.LicenseType{
			Id:           pair.licenseType,
			MaxSupply:    math.ZeroInt(),
			IssuedCount:  math.ZeroInt(),
			ActiveCount:  math.ZeroInt(),
			RevokedCount: math.ZeroInt(),
		}))
		require.NoError(t, nk.NodeTypes.Set(ctx, pair.nodeType, networktypes.NodeType{
			Id:            pair.nodeType,
			LicenseTypeId: pair.licenseType,
		}))
		require.NoError(t, nk.NodeTypeByLicenseType.Set(ctx, pair.licenseType, pair.nodeType))
	}

	return &NetworkFixture{
		Keeper:          nk,
		LicenseKeeper:   lk,
		AccountKeeper:   ak,
		BankKeeper:      bk,
		Ctx:             ctx,
		Authority:       authority,
		Owner:           owner,
		NodeType:        trustNodeType,
		LicenseType:     trustLicenseType,
		NanoNodeType:    nanoNodeType,
		NanoLicenseType: nanoLicenseType,
	}
}

// RegisterNodeType writes a node type record and its by-license-type mapping
// directly into network state, bypassing the grant-gated msg path. Tests that
// exercise that gate drive the msg server instead.
//
// The one-to-one invariant is asserted rather than assumed: this helper skips
// the handler that normally enforces it, so without the check a test could
// quietly overwrite an existing binding and then assert against state the
// chain could never reach.
func (f *NetworkFixture) RegisterNodeType(t testing.TB, id, licenseTypeID string) {
	t.Helper()
	bound, err := f.Keeper.NodeTypeByLicenseType.Get(f.Ctx, licenseTypeID)
	require.ErrorIsf(t, err, collections.ErrNotFound,
		"license type %s already backs node type %s; each license type may back only one", licenseTypeID, bound)

	require.NoError(t, f.Keeper.NodeTypes.Set(f.Ctx, id, networktypes.NodeType{
		Id:            id,
		LicenseTypeId: licenseTypeID,
	}))
	require.NoError(t, f.Keeper.NodeTypeByLicenseType.Set(f.Ctx, licenseTypeID, id))
}

// GrantNetwork writes a module-wide network grant directly into the grant
// store, bypassing the owner-gated msg path.
func (f *NetworkFixture) GrantNetwork(t testing.TB, grantee, action string) {
	t.Helper()
	require.NoError(t, f.Keeper.Grants.Set(f.Ctx, grantee, action, ""))
}

// IssueLicenses issues count active licenses of LicenseType — the trust pair —
// to holder, directly through license state (the license msg path is covered by
// the license module's own tests). Use IssueLicensesOfType for NanoLicenseType
// or any other type; entitlement is per node type, so which type is issued
// decides which nodes the holder may run.
func (f *NetworkFixture) IssueLicenses(t testing.TB, holder string, count uint64) {
	t.Helper()
	f.IssueLicensesOfType(t, holder, f.LicenseType, count)
}

// IssueLicensesOfType is IssueLicenses for an arbitrary license type id.
func (f *NetworkFixture) IssueLicensesOfType(t testing.TB, holder, typeID string, count uint64) {
	t.Helper()
	SeedActiveLicenses(t, f.LicenseKeeper, f.Ctx, holder, typeID, count)
}

// RevokeLicenses revokes count of holder's active licenses of the fixture's
// counted type through the license msg server, exercising the real
// revocation path (holder-index removal included). The most recently issued
// licenses are revoked first.
func (f *NetworkFixture) RevokeLicenses(t testing.TB, holder string, count uint64) {
	t.Helper()

	revoker := sample.AccAddress()
	require.NoError(t, f.LicenseKeeper.Grants.Set(f.Ctx, revoker, licensetypes.ActionRevoke, f.LicenseType))

	rng := collections.NewSuperPrefixedTripleRangeReversed[string, string, uint64](holder, f.LicenseType)
	var ids []uint64
	require.NoError(t, f.LicenseKeeper.ActiveLicensesByHolder.Walk(f.Ctx, rng, func(key collections.Triple[string, string, uint64]) (bool, error) {
		ids = append(ids, key.K3())
		return uint64(len(ids)) >= count, nil
	}))
	require.Len(t, ids, int(count), "holder %s does not have %d active license(s) of type %s", holder, count, f.LicenseType)

	_, err := licensekeeper.NewMsgServerImpl(f.LicenseKeeper).RevokeLicenses(f.Ctx, &licensetypes.MsgRevokeLicenses{
		Revoker:       revoker,
		LicenseTypeId: f.LicenseType,
		LicenseIds:    ids,
	})
	require.NoError(t, err)
}

// WithBlockTime returns the fixture context advanced to the given block time
// and the next height.
func (f *NetworkFixture) WithBlockTime(t time.Time) sdk.Context {
	header := f.Ctx.BlockHeader()
	header.Time = t
	header.Height++
	return f.Ctx.WithBlockHeader(header)
}
