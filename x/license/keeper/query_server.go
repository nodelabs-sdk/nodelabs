package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"github.com/cosmos/cosmos-sdk/types/query"

	accesskeeper "github.com/nodelabs-sdk/nodelabs/x/access/keeper"
	"github.com/nodelabs-sdk/nodelabs/x/license/types"
)

var _ types.QueryServer = Querier{}

type Querier struct {
	Keeper
}

func NewQuerier(keeper Keeper) Querier {
	return Querier{Keeper: keeper}
}

// Params returns the module parameters. An unset owner comes back empty rather
// than as an error: "no owner configured yet" is a legitimate chain state.
func (q Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.GetParams(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

// Actions returns the module's action vocabulary. It is compiled in rather
// than stored, so this reads no state.
func (q Querier) Actions(_ context.Context, _ *types.QueryActionsRequest) (*types.QueryActionsResponse, error) {
	spec := q.Keeper.Grants.Spec()
	return &types.QueryActionsResponse{
		Actions:  spec.SortedActions(),
		Unscoped: spec.SortedUnscoped(),
	}, nil
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

// GrantsByScope is a filtered walk rather than a prefix read: scope is the last
// key component. Prefer GrantsByGrantee or Can on hot paths.
func (q Querier) GrantsByScope(ctx context.Context, req *types.QueryGrantsByScopeRequest) (*types.QueryGrantsByScopeResponse, error) {
	grants, pageResp, err := accesskeeper.PaginateByScope(ctx, q.Keeper.Grants, req.Scope, req.Action, req.Pagination)
	if err != nil {
		return nil, err
	}
	return &types.QueryGrantsByScopeResponse{Grants: grants, Pagination: pageResp}, nil
}

func (q Querier) Can(ctx context.Context, req *types.QueryCanRequest) (*types.QueryCanResponse, error) {
	can, err := q.Keeper.Grants.HasGrant(ctx, req.Grantee, req.Action, req.Scope)
	if err != nil {
		return nil, err
	}
	return &types.QueryCanResponse{Can: can}, nil
}

func (q Querier) LicenseType(ctx context.Context, req *types.QueryLicenseTypeRequest) (*types.QueryLicenseTypeResponse, error) {
	lt, err := q.Keeper.LicenseTypes.Get(ctx, req.Id)
	if err != nil {
		return nil, types.ErrLicenseTypeNotFound.Wrapf("license type %s not found", req.Id)
	}
	return &types.QueryLicenseTypeResponse{LicenseType: lt}, nil
}

func (q Querier) LicenseTypes(ctx context.Context, req *types.QueryLicenseTypesRequest) (*types.QueryLicenseTypesResponse, error) {
	results, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.LicenseTypes, req.Pagination,
		func(_ string, lt types.LicenseType) (types.LicenseType, error) {
			return lt, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryLicenseTypesResponse{LicenseTypes: results, Pagination: pageResp}, nil
}

func (q Querier) License(ctx context.Context, req *types.QueryLicenseRequest) (*types.QueryLicenseResponse, error) {
	l, err := q.Keeper.Licenses.Get(ctx, req.Id)
	if err != nil {
		return nil, types.ErrLicenseNotFound.Wrapf("license %d not found", req.Id)
	}
	return &types.QueryLicenseResponse{License: l}, nil
}

// Licenses returns every license across all license types, active and
// revoked, paginated over the id key space in ascending id order.
func (q Querier) Licenses(ctx context.Context, req *types.QueryLicensesRequest) (*types.QueryLicensesResponse, error) {
	licenses, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.Licenses, req.Pagination,
		func(_ uint64, l types.License) (types.License, error) {
			return l, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryLicensesResponse{Licenses: licenses, Pagination: pageResp}, nil
}

// withTriplePrefix constrains pagination over a triple-keyed collection to
// keys whose first component equals k1.
func withTriplePrefix[K1, K2, K3 any](k1 K1) func(o *query.CollectionsPaginateOptions[collections.Triple[K1, K2, K3]]) {
	return func(o *query.CollectionsPaginateOptions[collections.Triple[K1, K2, K3]]) {
		prefix := collections.TriplePrefix[K1, K2, K3](k1)
		o.Prefix = &prefix
	}
}

// withTripleSuperPrefix constrains pagination over a triple-keyed collection
// to keys whose first two components equal (k1, k2).
func withTripleSuperPrefix[K1, K2, K3 any](k1 K1, k2 K2) func(o *query.CollectionsPaginateOptions[collections.Triple[K1, K2, K3]]) {
	return func(o *query.CollectionsPaginateOptions[collections.Triple[K1, K2, K3]]) {
		prefix := collections.TripleSuperPrefix[K1, K2, K3](k1, k2)
		o.Prefix = &prefix
	}
}

// LicensesByType returns every license of one type, active and revoked,
// resolved through the by-type index since licenses are keyed by id alone.
func (q Querier) LicensesByType(ctx context.Context, req *types.QueryLicensesByTypeRequest) (*types.QueryLicensesByTypeResponse, error) {
	licenses, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.LicensesByType, req.Pagination,
		func(key collections.Pair[string, uint64], _ collections.NoValue) (types.License, error) {
			return q.Keeper.Licenses.Get(ctx, key.K2())
		},
		query.WithCollectionPaginationPairPrefix[string, uint64](req.TypeId),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryLicensesByTypeResponse{Licenses: licenses, Pagination: pageResp}, nil
}

// LicensesByHolder returns the holder's active licenses. Revoked licenses are
// not indexed by holder; they remain reachable via License / LicensesByType.
func (q Querier) LicensesByHolder(ctx context.Context, req *types.QueryLicensesByHolderRequest) (*types.QueryLicensesByHolderResponse, error) {
	licenses, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.ActiveLicensesByHolder, req.Pagination,
		func(key collections.Triple[string, string, uint64], _ collections.NoValue) (types.License, error) {
			return q.Keeper.Licenses.Get(ctx, key.K3())
		},
		withTriplePrefix[string, string, uint64](req.Holder),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryLicensesByHolderResponse{Licenses: licenses, Pagination: pageResp}, nil
}

// LicensesByHolderAndType returns the holder's active licenses of one type.
// Revoked licenses are not indexed by holder; they remain reachable via
// License / LicensesByType.
func (q Querier) LicensesByHolderAndType(ctx context.Context, req *types.QueryLicensesByHolderAndTypeRequest) (*types.QueryLicensesByHolderAndTypeResponse, error) {
	licenses, pageResp, err := query.CollectionPaginate(ctx, q.Keeper.ActiveLicensesByHolder, req.Pagination,
		func(key collections.Triple[string, string, uint64], _ collections.NoValue) (types.License, error) {
			return q.Keeper.Licenses.Get(ctx, key.K3())
		},
		withTripleSuperPrefix[string, string, uint64](req.Holder, req.TypeId),
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryLicensesByHolderAndTypeResponse{Licenses: licenses, Pagination: pageResp}, nil
}
