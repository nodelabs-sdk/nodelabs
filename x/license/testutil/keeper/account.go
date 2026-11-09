package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
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
