package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// TestGenesisRoundTrip builds state through the msg server, exports it,
// imports it into a fresh fixture, and checks both the re-export and the
// derived structures InitGenesis rebuilds.
func TestGenesisRoundTrip(t *testing.T) {
	f, ms := setup(t)
	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 1)

	key := authorizeKey(t, f, ms, f.Ctx, operator)
	disabledKey := authorizeKey(t, f, ms, f.Ctx, operator)
	nodeA := activateNode(t, f, ms, f.Ctx, key, operator)
	nodeB := activateNode(t, f, ms, f.Ctx, key, operator)

	_, err := ms.DeauthorizeActivationKey(f.Ctx, &types.MsgDeauthorizeActivationKey{
		Operator:          operator,
		ActivationAddress: disabledKey,
	})
	require.NoError(t, err)
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: nodeB})
	require.NoError(t, err)

	// A day-two touch so nodes sit in different day buckets.
	later := f.WithBlockTime(keepertest.NetworkFixtureBlockTime.Add(24 * time.Hour))
	require.NoError(t, f.Keeper.TouchNodeActivity(later, nodeA))
	_, err = ms.UpdateNodeStatus(later, &types.MsgUpdateNodeStatus{NodeAddress: nodeA})
	require.NoError(t, err)

	// Burn gasless quota so the export carries counters of both key forms: the
	// activate kind is scoped to its node type, the rest take the empty scope.
	// A scope dropped in transit would silently hand a key a fresh activation
	// allowance on every export/import.
	require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(later, &types.MsgActivateNode{
		ActivationAddress: key, Operator: operator,
		NodeAddress: sample.AccAddress(), NodeType: f.NodeType,
	}))
	require.NoError(t, f.Keeper.CheckAndConsumeGaslessQuota(later, &types.MsgAuthorizeActivationKey{
		Operator: operator, ActivationAddress: sample.AccAddress(),
	}))

	exported := f.Keeper.ExportGenesis(f.Ctx)
	require.NoError(t, exported.Validate())

	// Import into a fresh fixture.
	g, _ := setup(t)
	require.NoError(t, g.Keeper.InitGenesis(g.Ctx, exported))

	// The re-export must match.
	require.Equal(t, exported, g.Keeper.ExportGenesis(g.Ctx))

	// Derived structures are rebuilt: operator registration, node counts,
	// and one recent-activity entry per node in its last-active bucket.
	registered, err := g.Keeper.Operators.Has(g.Ctx, operator)
	require.NoError(t, err)
	require.True(t, registered)

	counts, err := g.Keeper.GetOperatorNodeCounts(g.Ctx, operator, g.NodeType)
	require.NoError(t, err)
	require.Equal(t, uint64(2), counts.Total)
	require.Equal(t, uint64(1), counts.Active)

	day0 := types.DayEpoch(keepertest.NetworkFixtureBlockTime)
	require.Equal(t, map[string]uint64{nodeA: day0 + 1, nodeB: day0}, recentEntries(t, g, operator))

	// The activate counter lands under its node-type scope rather than pooled,
	// and the unscoped kinds keep the empty scope. The re-export comparison
	// above would catch a lost scope; this pins which key form each kind uses.
	activateCounter, err := g.Keeper.GaslessCounters.Get(g.Ctx,
		collections.Join3(types.GaslessKindActivate, key, g.NodeType))
	require.NoError(t, err)
	require.Equal(t, uint64(1), activateCounter.DailyCount)

	_, err = g.Keeper.GaslessCounters.Get(g.Ctx,
		collections.Join3(types.GaslessKindActivate, key, types.GaslessScopeNone))
	require.ErrorIs(t, err, collections.ErrNotFound, "activate counters must not be pooled under the empty scope")

	authorizeCounter, err := g.Keeper.GaslessCounters.Get(g.Ctx,
		collections.Join3(types.GaslessKindAuthorize, operator, types.GaslessScopeNone))
	require.NoError(t, err)
	require.Equal(t, uint64(1), authorizeCounter.DailyCount)

	// Node types survive, and the by-license-type mapping is rebuilt rather
	// than exported — so a missing rebuild would leave the filtered query empty
	// while the re-export above still matched.
	require.ElementsMatch(t, []string{f.NodeType, f.NanoNodeType}, idsOf(exported.NodeTypes))
	for _, nt := range exported.NodeTypes {
		bound, err := g.Keeper.NodeTypeByLicenseType.Get(g.Ctx, nt.LicenseTypeId)
		require.NoError(t, err, "mapping missing for license type %s", nt.LicenseTypeId)
		require.Equal(t, nt.Id, bound)
	}
}

func TestGenesisValidate(t *testing.T) {
	operator := sample.AccAddress()
	keyAddr := sample.AccAddress()
	nodeAddr := sample.AccAddress()
	now := keepertest.NetworkFixtureBlockTime

	validKey := types.ActivationKey{Address: keyAddr, Operator: operator, CreatedAt: now, Status: types.KeyActive}
	validNode := types.Node{Address: nodeAddr, Operator: operator, ActivatedBy: keyAddr, Type: "nodelabs.trust", Status: types.NodeActive, LastActiveTime: now}
	validNodeType := types.NodeType{Id: "nodelabs.trust", LicenseTypeId: "nodelabs.node.trust"}

	tests := []struct {
		name      string
		mutate    func(*types.GenesisState)
		expErrMsg string
	}{
		{
			name:   "valid",
			mutate: func(*types.GenesisState) {},
		},
		{
			name: "duplicate node",
			mutate: func(gs *types.GenesisState) {
				gs.Nodes = append(gs.Nodes, gs.Nodes[0])
			},
			expErrMsg: "duplicate node",
		},
		{
			name: "duplicate activation key",
			mutate: func(gs *types.GenesisState) {
				gs.ActivationKeys = append(gs.ActivationKeys, gs.ActivationKeys[0])
			},
			expErrMsg: "duplicate activation key",
		},
		{
			name: "node with unknown activation key",
			mutate: func(gs *types.GenesisState) {
				gs.Nodes[0].ActivatedBy = sample.AccAddress()
			},
			expErrMsg: "not a listed activation key",
		},
		{
			name: "node operator mismatch with its key",
			mutate: func(gs *types.GenesisState) {
				other := sample.AccAddress()
				gs.Nodes[0].Operator = other
			},
			expErrMsg: "bound to operator",
		},
		{
			name: "counter for unknown node",
			mutate: func(gs *types.GenesisState) {
				gs.NodeStatusCounters = append(gs.NodeStatusCounters, types.NodeStatusCounter{
					NodeAddress: sample.AccAddress(),
					Counter:     types.ActivityCounter{LatestTime: now, DailyCount: 1},
				})
			},
			expErrMsg: "unknown node",
		},
		{
			name: "self-bound activation key",
			mutate: func(gs *types.GenesisState) {
				gs.ActivationKeys[0].Operator = gs.ActivationKeys[0].Address
				gs.Nodes = nil
			},
			expErrMsg: "equals its operator",
		},
		{
			name: "invalid params",
			mutate: func(gs *types.GenesisState) {
				gs.Params.ActivationLimitMultiplier = 0
			},
			expErrMsg: "activation_limit_multiplier",
		},
		{
			name: "duplicate node type",
			mutate: func(gs *types.GenesisState) {
				gs.NodeTypes = append(gs.NodeTypes, gs.NodeTypes[0])
			},
			expErrMsg: "duplicate node type",
		},
		{
			// The registry is what activation resolves a node's type through,
			// so importing a node whose type is absent would strand it.
			name: "node with unregistered type",
			mutate: func(gs *types.GenesisState) {
				gs.Nodes[0].Type = "never.registered"
			},
			expErrMsg: "is not a listed node type",
		},
		{
			name: "node type missing license type",
			mutate: func(gs *types.GenesisState) {
				gs.NodeTypes[0].LicenseTypeId = ""
			},
			expErrMsg: "license_type_id must not be empty",
		},
		{
			// Two node types on one license type would collide in the derived
			// mapping, so one would silently win on import.
			name: "two node types bound to one license type",
			mutate: func(gs *types.GenesisState) {
				gs.NodeTypes = append(gs.NodeTypes, types.NodeType{
					Id:            "second.node",
					LicenseTypeId: gs.NodeTypes[0].LicenseTypeId,
				})
			},
			expErrMsg: "both bound to license type",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := types.GenesisState{
				Params:         types.DefaultParams(),
				NodeTypes:      []types.NodeType{validNodeType},
				Nodes:          []types.Node{validNode},
				ActivationKeys: []types.ActivationKey{validKey},
			}
			tc.mutate(&gs)
			err := gs.Validate()
			if tc.expErrMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.expErrMsg)
		})
	}
}

// TestGenesisRoundTripOwnerAndGrants covers the ownership and access state:
// the owner is a parameter and its grants export and re-import with the rest
// of the module, rather than living in a separate genesis section.
func TestGenesisRoundTripOwnerAndGrants(t *testing.T) {
	f := keepertest.NewNetworkFixture(t)
	grantee := sample.AccAddress()

	f.GrantNetwork(t, grantee, types.ActionWalletCreate)
	f.GrantNetwork(t, grantee, types.ActionNodeTypeCreate)

	exported := f.Keeper.ExportGenesis(f.Ctx)
	require.Equal(t, f.Owner, exported.Params.Owner)
	require.ElementsMatch(t, []accesstypes.Grant{
		{Grantee: grantee, Action: types.ActionWalletCreate},
		{Grantee: grantee, Action: types.ActionNodeTypeCreate},
	}, exported.Grants)

	fresh := keepertest.NewNetworkFixture(t)
	require.NoError(t, fresh.Keeper.InitGenesis(fresh.Ctx, exported))

	params, err := fresh.Keeper.GetParams(fresh.Ctx)
	require.NoError(t, err)
	require.Equal(t, f.Owner, params.Owner)

	has, err := fresh.Keeper.Grants.HasGrant(fresh.Ctx, grantee, types.ActionWalletCreate, "")
	require.NoError(t, err)
	require.True(t, has)
}

// Stateless genesis validation enforces this module's access rules: the action
// must be in the vocabulary and must carry no scope.
func TestGenesisGrantValidation(t *testing.T) {
	addr := sample.AccAddress()

	base := func(grants []accesstypes.Grant) types.GenesisState {
		gs := *types.DefaultGenesis()
		gs.Grants = grants
		return gs
	}

	require.NoError(t, base([]accesstypes.Grant{
		{Grantee: addr, Action: types.ActionWalletCreate},
	}).Validate())

	require.ErrorContains(t, base([]accesstypes.Grant{
		{Grantee: addr, Action: "node.destroy"},
	}).Validate(), "not part of this module's vocabulary")

	require.ErrorContains(t, base([]accesstypes.Grant{
		{Grantee: addr, Action: types.ActionWalletCreate, Scope: "anything"},
	}).Validate(), "scope must be empty")

	require.ErrorContains(t, base([]accesstypes.Grant{
		{Grantee: "not-an-address", Action: types.ActionWalletCreate},
	}).Validate(), "invalid grantee address")

	g := accesstypes.Grant{Grantee: addr, Action: types.ActionWalletCreate}
	require.ErrorContains(t, base([]accesstypes.Grant{g, g}).Validate(), "duplicate grant")
}
