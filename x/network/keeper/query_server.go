package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"

	accesskeeper "github.com/nodelabs-sdk/nodelabs/x/access/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

var _ types.QueryServer = Querier{}

type Querier struct {
	Keeper
}

func NewQuerier(keeper Keeper) Querier {
	return Querier{Keeper: keeper}
}

func (q Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

// Actions returns the module's action vocabulary. It is compiled in rather
// than stored, so this reads no state. There is no unscoped list: this module
// does not scope its actions, so every action is module-wide.
func (q Querier) Actions(_ context.Context, _ *types.QueryActionsRequest) (*types.QueryActionsResponse, error) {
	return &types.QueryActionsResponse{Actions: q.Keeper.Grants.Spec().SortedActions()}, nil
}

func (q Querier) Grants(ctx context.Context, req *types.QueryGrantsRequest) (*types.QueryGrantsResponse, error) {
	grants, pageResp, err := accesskeeper.PaginateAll(ctx, q.Keeper.Grants, req.Pagination)
	if err != nil {
		return nil, err
	}
	return &types.QueryGrantsResponse{Grants: grants, Pagination: pageResp}, nil
}

func (q Querier) GrantsByGrantee(ctx context.Context, req *types.QueryGrantsByGranteeRequest) (*types.QueryGrantsByGranteeResponse, error) {
	grants, pageResp, err := accesskeeper.PaginateByGrantee(ctx, q.Keeper.Grants, req.Grantee, req.Pagination)
	if err != nil {
		return nil, err
	}
	return &types.QueryGrantsByGranteeResponse{Grants: grants, Pagination: pageResp}, nil
}

// Can reports whether a grantee holds an action grant. Grants here are always
// module-wide, so the lookup uses the empty scope.
func (q Querier) Can(ctx context.Context, req *types.QueryCanRequest) (*types.QueryCanResponse, error) {
	can, err := q.Keeper.Grants.HasGrant(ctx, req.Grantee, req.Action, "")
	if err != nil {
		return nil, err
	}
	return &types.QueryCanResponse{Can: can}, nil
}

func (q Querier) Node(ctx context.Context, req *types.QueryNodeRequest) (*types.QueryNodeResponse, error) {
	node, err := q.Keeper.Nodes.Get(ctx, req.Address)
	if err != nil {
		return nil, types.ErrNodeNotFound.Wrapf("node %s not found", req.Address)
	}
	return &types.QueryNodeResponse{Node: node}, nil
}

// matchesStatus reports whether a node passes a request's status filter. The
// unspecified zero value means "no filter": it is never a valid stored status,
// so it cannot collide with a real one, and a request that omits the field
// therefore keeps the original unfiltered behaviour.
func matchesStatus(node types.Node, want types.NodeStatus) bool {
	return want == types.NodeStatus_NODE_STATUS_UNSPECIFIED || node.Status == want
}

// Nodes returns every node record, optionally filtered by status. Records are
// retained after deactivation, so an unfiltered listing includes every node
// ever activated.
func (q Querier) Nodes(ctx context.Context, req *types.QueryNodesRequest) (*types.QueryNodesResponse, error) {
	nodes, pageResp, err := query.CollectionFilteredPaginate(ctx, q.Keeper.Nodes, req.Pagination,
		func(_ string, node types.Node) (bool, error) {
			return matchesStatus(node, req.Status), nil
		},
		func(_ string, node types.Node) (types.Node, error) {
			return node, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryNodesResponse{Nodes: nodes, Pagination: pageResp}, nil
}

// NodesByOperator returns the operator's node records, optionally filtered by
// status. The status filter narrows within the operator prefix; it never
// widens the scan beyond that operator.
func (q Querier) NodesByOperator(ctx context.Context, req *types.QueryNodesByOperatorRequest) (*types.QueryNodesByOperatorResponse, error) {
	nodes, pageResp, err := query.CollectionFilteredPaginate(ctx, q.Keeper.OperatorNodes, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (bool, error) {
			node, err := q.Keeper.Nodes.Get(ctx, key.K2())
			if err != nil {
				return false, err
			}
			return matchesStatus(node, req.Status), nil
		},
		func(key collections.Pair[string, string], _ collections.NoValue) (types.Node, error) {
			return q.Keeper.Nodes.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Operator),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryNodesByOperatorResponse{Nodes: nodes, Pagination: pageResp}, nil
}

// NodeType returns a registered node type by id.
func (q Querier) NodeType(ctx context.Context, req *types.QueryNodeTypeRequest) (*types.QueryNodeTypeResponse, error) {
	nt, err := q.Keeper.NodeTypes.Get(ctx, req.Id)
	if err != nil {
		return nil, types.ErrNodeTypeNotFound.Wrapf("node type %s not found", req.Id)
	}
	return &types.QueryNodeTypeResponse{NodeType: nt}, nil
}

// NodeTypes returns registered node types. When license_type_id is set the
// binding is one-to-one, so the answer is a single direct lookup and the
// result holds at most one node type; the list shape is kept so callers do not
// need a second response type for the filtered case. When it is empty every
// node type is returned, paginated.
func (q Querier) NodeTypes(ctx context.Context, req *types.QueryNodeTypesRequest) (*types.QueryNodeTypesResponse, error) {
	if req.LicenseTypeId != "" {
		id, err := q.Keeper.NodeTypeByLicenseType.Get(ctx, req.LicenseTypeId)
		if errors.Is(err, collections.ErrNotFound) {
			// No node type is bound to this license type. That is an empty
			// result, not an error: the license type may simply be unused.
			return &types.QueryNodeTypesResponse{}, nil
		}
		if err != nil {
			return nil, err
		}
		nt, err := q.Keeper.NodeTypes.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		return &types.QueryNodeTypesResponse{NodeTypes: []types.NodeType{nt}}, nil
	}

	nodeTypes, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.NodeTypes, req.Pagination,
		func(_ string, nt types.NodeType) (types.NodeType, error) {
			return nt, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryNodeTypesResponse{NodeTypes: nodeTypes, Pagination: pageResp}, nil
}

func (q Querier) ActivationKey(ctx context.Context, req *types.QueryActivationKeyRequest) (*types.QueryActivationKeyResponse, error) {
	key, err := q.Keeper.ActivationKeys.Get(ctx, req.Address)
	if err != nil {
		return nil, types.ErrKeyNotFound.Wrapf("activation key %s not found", req.Address)
	}
	return &types.QueryActivationKeyResponse{ActivationKey: key}, nil
}

func (q Querier) ActivationKeys(ctx context.Context, req *types.QueryActivationKeysRequest) (*types.QueryActivationKeysResponse, error) {
	keys, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.OperatorActivationKeys, req.Pagination,
		func(key collections.Pair[string, string], _ collections.NoValue) (types.ActivationKey, error) {
			return q.Keeper.ActivationKeys.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[string, string](req.Operator),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryActivationKeysResponse{ActivationKeys: keys, Pagination: pageResp}, nil
}

// NodeCounts returns the operator's node tallies alongside the limits their
// licenses imply, so a hosting provider can pre-check whether an activation
// would be admitted. Entitlement is per node type, so the answer is a
// breakdown: one entry per registered node type, or just the requested one.
func (q Querier) NodeCounts(ctx context.Context, req *types.QueryNodeCountsRequest) (*types.QueryNodeCountsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime()

	tally := func(nt types.NodeType) (types.NodeTypeCounts, error) {
		counts, err := q.Keeper.GetOperatorNodeCounts(ctx, req.Operator, nt.Id)
		if err != nil {
			return types.NodeTypeCounts{}, err
		}
		recent, err := q.Keeper.CountRecentActiveNodes(ctx, req.Operator, nt.Id, blockTime)
		if err != nil {
			return types.NodeTypeCounts{}, err
		}
		// stopAt 0: a query, so the full count is wanted rather than the
		// decision-bounded walk the handlers use.
		licenses, err := q.Keeper.licenseKeeper.CountActiveLicenses(ctx, req.Operator, []string{nt.LicenseTypeId}, 0)
		if err != nil {
			return types.NodeTypeCounts{}, err
		}
		return types.NodeTypeCounts{
			NodeType:        nt.Id,
			LicenseTypeId:   nt.LicenseTypeId,
			Total:           counts.Total,
			Active:          counts.Active,
			RecentActive:    recent,
			LicenseCount:    licenses,
			ActivationLimit: params.ActivationLimitMultiplier * licenses,
			SpamLimit:       params.SpamLimitMultiplier * licenses,
		}, nil
	}

	if req.NodeType != "" {
		nt, err := q.Keeper.NodeTypes.Get(ctx, req.NodeType)
		if err != nil {
			return nil, types.ErrNodeTypeNotFound.Wrapf("node type %s not found", req.NodeType)
		}
		entry, err := tally(nt)
		if err != nil {
			return nil, err
		}
		return &types.QueryNodeCountsResponse{Counts: []types.NodeTypeCounts{entry}}, nil
	}

	var out []types.NodeTypeCounts
	if err := q.Keeper.NodeTypes.Walk(ctx, nil, func(_ string, nt types.NodeType) (bool, error) {
		entry, err := tally(nt)
		if err != nil {
			return true, err
		}
		out = append(out, entry)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryNodeCountsResponse{Counts: out}, nil
}
