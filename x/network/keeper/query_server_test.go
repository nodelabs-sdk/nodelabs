package keeper_test

import (
	"sort"
	"testing"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"

	keepertest "github.com/nodelabs-sdk/nodelabs/testutil/keeper"
	"github.com/nodelabs-sdk/nodelabs/testutil/sample"
	"github.com/nodelabs-sdk/nodelabs/x/network/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// nodeFleet activates count nodes for a fresh operator and returns the
// operator plus the node addresses in activation order.
func nodeFleet(t testing.TB, f *keepertest.NetworkFixture, ms types.MsgServer, count int) (string, []string) {
	t.Helper()

	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, uint64(count))
	key := authorizeKey(t, f, ms, f.Ctx, operator)

	addrs := make([]string, 0, count)
	for i := 0; i < count; i++ {
		addrs = append(addrs, activateNode(t, f, ms, f.Ctx, key, operator))
	}
	return operator, addrs
}

// deactivate takes a node out of service through the real msg path.
func deactivate(t testing.TB, f *keepertest.NetworkFixture, ms types.MsgServer, operator, node string) {
	t.Helper()
	_, err := ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{
		Signer:      operator,
		NodeAddress: node,
	})
	require.NoError(t, err)
}

// addressesOf extracts node addresses so assertions read in terms of identity
// rather than whole records.
func addressesOf(nodes []types.Node) []string {
	out := make([]string, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, n.Address)
	}
	return out
}

// TestQueryNodesStatusFilter: an unset status keeps the original behaviour of
// listing every node ever activated, and each concrete status selects only
// its own.
func TestQueryNodesStatusFilter(t *testing.T) {
	f, ms := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	operator, addrs := nodeFleet(t, f, ms, 3)
	deactivate(t, f, ms, operator, addrs[1])

	// Unset status: deactivated records are retained, so all three come back.
	all, err := q.Nodes(f.Ctx, &types.QueryNodesRequest{})
	require.NoError(t, err)
	require.ElementsMatch(t, addrs, addressesOf(all.Nodes))

	active, err := q.Nodes(f.Ctx, &types.QueryNodesRequest{Status: types.NodeActive})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{addrs[0], addrs[2]}, addressesOf(active.Nodes))

	gone, err := q.Nodes(f.Ctx, &types.QueryNodesRequest{Status: types.NodeDeactivated})
	require.NoError(t, err)
	require.Equal(t, []string{addrs[1]}, addressesOf(gone.Nodes))
	require.Equal(t, types.NodeDeactivated, gone.Nodes[0].Status)
}

// TestQueryNodesByOperatorStatusFilter: the filter narrows within the operator
// prefix and never widens past it.
func TestQueryNodesByOperatorStatusFilter(t *testing.T) {
	f, ms := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	operator, addrs := nodeFleet(t, f, ms, 3)
	deactivate(t, f, ms, operator, addrs[1])

	// A second operator whose nodes must never appear in the first's results.
	otherOp, otherAddrs := nodeFleet(t, f, ms, 2)

	all, err := q.NodesByOperator(f.Ctx, &types.QueryNodesByOperatorRequest{Operator: operator})
	require.NoError(t, err)
	require.ElementsMatch(t, addrs, addressesOf(all.Nodes))

	active, err := q.NodesByOperator(f.Ctx, &types.QueryNodesByOperatorRequest{
		Operator: operator,
		Status:   types.NodeActive,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{addrs[0], addrs[2]}, addressesOf(active.Nodes))

	// The other operator's nodes are all active, so an active-only query for
	// the first operator must still not leak them.
	require.NotContains(t, addressesOf(active.Nodes), otherAddrs[0])

	gone, err := q.NodesByOperator(f.Ctx, &types.QueryNodesByOperatorRequest{
		Operator: operator,
		Status:   types.NodeDeactivated,
	})
	require.NoError(t, err)
	require.Equal(t, []string{addrs[1]}, addressesOf(gone.Nodes))

	// The second operator is unaffected by the first's deactivation.
	otherActive, err := q.NodesByOperator(f.Ctx, &types.QueryNodesByOperatorRequest{
		Operator: otherOp,
		Status:   types.NodeActive,
	})
	require.NoError(t, err)
	require.ElementsMatch(t, otherAddrs, addressesOf(otherActive.Nodes))
}

// TestQueryNodesFilteredPagination is the real risk in filtered pagination:
// the page key advances over the underlying keyspace, not over matches, so
// paging must still yield every match exactly once and never a filtered-out
// node — including when a page boundary lands next to one.
func TestQueryNodesFilteredPagination(t *testing.T) {
	f, ms := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	operator, addrs := nodeFleet(t, f, ms, 6)

	// Nodes are keyed by address, so store order is lexicographic. Deactivate
	// alternating nodes in that order, interleaving filtered-out records with
	// matches rather than clustering them at one end.
	byAddr := append([]string(nil), addrs...)
	sort.Strings(byAddr)

	var wantActive []string
	for i, addr := range byAddr {
		if i%2 == 1 {
			deactivate(t, f, ms, operator, addr)
			continue
		}
		wantActive = append(wantActive, addr)
	}
	require.Len(t, wantActive, 3)

	var got []string
	var nextKey []byte
	pages := 0
	for {
		page, err := q.Nodes(f.Ctx, &types.QueryNodesRequest{
			Status:     types.NodeActive,
			Pagination: &query.PageRequest{Key: nextKey, Limit: 2},
		})
		require.NoError(t, err)
		got = append(got, addressesOf(page.Nodes)...)
		pages++
		require.LessOrEqual(t, pages, 10, "pagination failed to terminate")

		nextKey = page.Pagination.NextKey
		if len(nextKey) == 0 {
			break
		}
	}

	require.Equal(t, wantActive, got, "every active node exactly once, in store order")
	// 3 matches at 2 per page: the assertion above is only meaningful if the
	// walk actually crossed a page boundary.
	require.Equal(t, 2, pages, "expected the filtered result set to span two pages")
}

// TestQueryNodesUnknownStatusMatchesNothing: a status outside the enum is not
// an error, it simply selects no nodes.
func TestQueryNodesUnknownStatusMatchesNothing(t *testing.T) {
	f, ms := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	nodeFleet(t, f, ms, 2)

	resp, err := q.Nodes(f.Ctx, &types.QueryNodesRequest{Status: types.NodeStatus(99)})
	require.NoError(t, err)
	require.Empty(t, resp.Nodes)
}

// ---------------------------------------------------------------------------
// NodeType / NodeTypes
// ---------------------------------------------------------------------------

// idsOf extracts node type ids so assertions read in terms of identity.
func idsOf(nodeTypes []types.NodeType) []string {
	out := make([]string, 0, len(nodeTypes))
	for _, nt := range nodeTypes {
		out = append(out, nt.Id)
	}
	return out
}

// TestQueryNodeType: a registered type comes back whole; an unregistered one is
// a typed not-found rather than an empty record.
func TestQueryNodeType(t *testing.T) {
	f, _ := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	resp, err := q.NodeType(f.Ctx, &types.QueryNodeTypeRequest{Id: f.NodeType})
	require.NoError(t, err)
	require.Equal(t, types.NodeType{
		Id:            f.NodeType,
		LicenseTypeId: f.LicenseType,
	}, resp.NodeType)

	_, err = q.NodeType(f.Ctx, &types.QueryNodeTypeRequest{Id: "never.registered"})
	require.ErrorIs(t, err, types.ErrNodeTypeNotFound)
}

// TestQueryNodeTypesLicenseFilter is the query the binding exists to serve:
// given a license type id, return the node type bound to it. The binding is
// one-to-one, so the answer is always zero or one record.
func TestQueryNodeTypesLicenseFilter(t *testing.T) {
	f, _ := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	// The fixture registers two node types, each on its own license type.
	// Each license type backs exactly one, so these never mix.
	all, err := q.NodeTypes(f.Ctx, &types.QueryNodeTypesRequest{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{f.NodeType, f.NanoNodeType}, idsOf(all.NodeTypes))

	mine, err := q.NodeTypes(f.Ctx, &types.QueryNodeTypesRequest{LicenseTypeId: f.LicenseType})
	require.NoError(t, err)
	require.Equal(t, []string{f.NodeType}, idsOf(mine.NodeTypes))

	// The lookup hydrates the full record, not just the id.
	require.Equal(t, f.NodeType, mine.NodeTypes[0].Id)
	require.Equal(t, f.LicenseType, mine.NodeTypes[0].LicenseTypeId)

	other, err := q.NodeTypes(f.Ctx, &types.QueryNodeTypesRequest{LicenseTypeId: f.NanoLicenseType})
	require.NoError(t, err)
	require.Equal(t, []string{f.NanoNodeType}, idsOf(other.NodeTypes))

	// A license type nobody bound to is empty, not an error.
	none, err := q.NodeTypes(f.Ctx, &types.QueryNodeTypesRequest{LicenseTypeId: "unused.license"})
	require.NoError(t, err)
	require.Empty(t, none.NodeTypes)
}

// ---------------------------------------------------------------------------
// NodeCounts
// ---------------------------------------------------------------------------

// TestQueryNodeCounts: limits are reported per node type and are derived from
// the operator's licenses of that type's bound license type alone.
func TestQueryNodeCounts(t *testing.T) {
	f, ms := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	operator := sample.AccAddress()
	f.IssueLicenses(t, operator, 2) // trust licences only; nano stays unlicensed
	key := authorizeKey(t, f, ms, f.Ctx, operator)
	node := activateNode(t, f, ms, f.Ctx, key, operator)

	byType := func(resp *types.QueryNodeCountsResponse) map[string]types.NodeTypeCounts {
		out := make(map[string]types.NodeTypeCounts, len(resp.Counts))
		for _, c := range resp.Counts {
			out[c.NodeType] = c
		}
		return out
	}

	all, err := q.NodeCounts(f.Ctx, &types.QueryNodeCountsRequest{Operator: operator})
	require.NoError(t, err)
	entries := byType(all)
	require.Len(t, entries, 2, "one entry per registered node type")

	// The licensed type: 2 licenses x multiplier 3 = 6, spam 9 x 2 = 18.
	mine := entries[f.NodeType]
	require.Equal(t, f.LicenseType, mine.LicenseTypeId)
	require.Equal(t, uint64(2), mine.LicenseCount)
	require.Equal(t, uint64(6), mine.ActivationLimit)
	require.Equal(t, uint64(18), mine.SpamLimit)
	require.Equal(t, uint64(1), mine.Total)
	require.Equal(t, uint64(1), mine.Active)
	require.Equal(t, uint64(1), mine.RecentActive)

	// The unlicensed type reports zeroes, not the other type's numbers.
	nano := entries[f.NanoNodeType]
	require.Equal(t, f.NanoLicenseType, nano.LicenseTypeId)
	require.Zero(t, nano.LicenseCount)
	require.Zero(t, nano.ActivationLimit)
	require.Zero(t, nano.Total)

	// Filtering returns just that type.
	one, err := q.NodeCounts(f.Ctx, &types.QueryNodeCountsRequest{
		Operator: operator,
		NodeType: f.NanoNodeType,
	})
	require.NoError(t, err)
	require.Len(t, one.Counts, 1)
	require.Equal(t, f.NanoNodeType, one.Counts[0].NodeType)

	// Deactivating drops active but not total, in the right type's tally.
	_, err = ms.DeactivateNode(f.Ctx, &types.MsgDeactivateNode{Signer: operator, NodeAddress: node})
	require.NoError(t, err)
	after, err := q.NodeCounts(f.Ctx, &types.QueryNodeCountsRequest{Operator: operator, NodeType: f.NodeType})
	require.NoError(t, err)
	require.Equal(t, uint64(1), after.Counts[0].Total)
	require.Zero(t, after.Counts[0].Active)

	_, err = q.NodeCounts(f.Ctx, &types.QueryNodeCountsRequest{Operator: operator, NodeType: "never.registered"})
	require.ErrorIs(t, err, types.ErrNodeTypeNotFound)
}

// TestQueryNodeTypesPagination: the unfiltered listing still walks the whole
// registry, so it must page correctly. (The filtered branch is a single keyed
// read and returns at most one record, so it has nothing to page.)
func TestQueryNodeTypesPagination(t *testing.T) {
	f, _ := setup(t)
	q := keeper.NewQuerier(f.Keeper)

	// Each on its own license type, as the one-to-one binding requires. These
	// sort ahead of the fixture's two nodelabs.* types.
	added := []string{"a.one", "b.two", "c.three"}
	for _, id := range added {
		f.RegisterNodeType(t, id, id+".license")
	}
	// Store order is by id, and "nodelabs.nano" sorts before "nodelabs.trust".
	want := append(append([]string{}, added...), f.NanoNodeType, f.NodeType)

	var got []string
	var nextKey []byte
	pages := 0
	for {
		page, err := q.NodeTypes(f.Ctx, &types.QueryNodeTypesRequest{
			Pagination: &query.PageRequest{Key: nextKey, Limit: 2},
		})
		require.NoError(t, err)
		got = append(got, idsOf(page.NodeTypes)...)
		pages++
		require.LessOrEqual(t, pages, 10, "pagination failed to terminate")

		nextKey = page.Pagination.NextKey
		if len(nextKey) == 0 {
			break
		}
	}

	require.Equal(t, want, got, "every node type exactly once, in id order")
	// 5 records at 2 per page: the assertion above only means something if the
	// walk actually crossed page boundaries.
	require.Equal(t, 3, pages, "expected the listing to span three pages")
}
