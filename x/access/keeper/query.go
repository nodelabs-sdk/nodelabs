package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/nodelabs-sdk/nodelabs/x/access/types"
)

// GrantFromKey rebuilds the flat Grant view from a state key.
func GrantFromKey(key GrantKey) types.Grant {
	return types.Grant{
		Grantee: key.K1(),
		Action:  key.K2(),
		Scope:   key.K3(),
	}
}

// withPrefix constrains pagination to keys whose first component equals k1.
func withPrefix[K1, K2, K3 any](k1 K1) func(o *query.CollectionsPaginateOptions[collections.Triple[K1, K2, K3]]) {
	return func(o *query.CollectionsPaginateOptions[collections.Triple[K1, K2, K3]]) {
		prefix := collections.TriplePrefix[K1, K2, K3](k1)
		o.Prefix = &prefix
	}
}

// PaginateAll pages over every grant in the module, in key order.
func PaginateAll(ctx context.Context, g Grants, pageReq *query.PageRequest) ([]types.Grant, *query.PageResponse, error) {
	return query.CollectionPaginate(ctx, g.KeySet(), pageReq,
		func(key GrantKey, _ collections.NoValue) (types.Grant, error) {
			return GrantFromKey(key), nil
		},
	)
}

// PaginateByGrantee pages over the grants held by one address. Grantee is the
// first key component, so this is a prefix read.
func PaginateByGrantee(ctx context.Context, g Grants, grantee string, pageReq *query.PageRequest) ([]types.Grant, *query.PageResponse, error) {
	return query.CollectionPaginate(ctx, g.KeySet(), pageReq,
		func(key GrantKey, _ collections.NoValue) (types.Grant, error) {
			return GrantFromKey(key), nil
		},
		withPrefix[string, string, string](grantee),
	)
}

// PaginateByScope pages over the grants that apply to a scope, optionally
// narrowed to a single action. Scope is the last key component, so this is a
// filtered walk of the whole store rather than a prefix read; prefer
// PaginateByGrantee or HasGrant on hot paths.
func PaginateByScope(ctx context.Context, g Grants, scope, action string, pageReq *query.PageRequest) ([]types.Grant, *query.PageResponse, error) {
	return query.CollectionFilteredPaginate(ctx, g.KeySet(), pageReq,
		func(key GrantKey, _ collections.NoValue) (bool, error) {
			if key.K3() != scope {
				return false, nil
			}
			if action != "" && key.K2() != action {
				return false, nil
			}
			return true, nil
		},
		func(key GrantKey, _ collections.NoValue) (types.Grant, error) {
			return GrantFromKey(key), nil
		},
	)
}
