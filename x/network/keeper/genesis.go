package keeper

import (
	"context"

	"cosmossdk.io/collections"

	"github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// InitGenesis initializes the module's state from a genesis state. The
// denormalized structures (operator set, operator indexes, node counts, the
// recent-activity index, and the node-type-by-license index) are derived from
// the primary records here, so they can never disagree with them.
func (k *Keeper) InitGenesis(ctx context.Context, data *types.GenesisState) error {
	if err := data.Validate(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, data.Params); err != nil {
		return err
	}

	if err := k.Grants.Init(ctx, data.Grants); err != nil {
		return err
	}

	// Node types first: nodes reference them, and Validate has already
	// confirmed every node's type is present here.
	for _, nt := range data.NodeTypes {
		if err := k.NodeTypes.Set(ctx, nt.Id, nt); err != nil {
			return err
		}
		if err := k.NodeTypeByLicenseType.Set(ctx, nt.LicenseTypeId, nt.Id); err != nil {
			return err
		}
	}

	for _, key := range data.ActivationKeys {
		if err := k.ActivationKeys.Set(ctx, key.Address, key); err != nil {
			return err
		}
		if err := k.OperatorActivationKeys.Set(ctx, collections.Join(key.Operator, key.Address)); err != nil {
			return err
		}
		if err := k.Operators.Set(ctx, key.Operator); err != nil {
			return err
		}
	}

	// Tallies are per (operator, nodeType), so the rebuild accumulates on that
	// pair rather than on the operator alone.
	//
	// The accumulator is keyed on a plain value struct, NOT collections.Pair:
	// Pair stores its parts as pointers, so as a Go map key it compares by
	// pointer identity and every Join would allocate a fresh, never-equal key.
	type tallyKey struct{ operator, nodeType string }
	counts := make(map[tallyKey]types.OperatorNodeCounts)
	for _, node := range data.Nodes {
		if err := k.Nodes.Set(ctx, node.Address, node); err != nil {
			return err
		}
		if err := k.OperatorNodes.Set(ctx, collections.Join(node.Operator, node.Address)); err != nil {
			return err
		}
		// Exactly one recent-activity entry per node, in the day bucket of
		// its last counted activity.
		if err := k.RecentNodeActivity.Set(ctx, collections.Join4(node.Operator, node.Type, types.DayEpoch(node.LastActiveTime), node.Address)); err != nil {
			return err
		}
		if err := k.Operators.Set(ctx, node.Operator); err != nil {
			return err
		}

		key := tallyKey{operator: node.Operator, nodeType: node.Type}
		c := counts[key]
		c.Total++
		if node.Status == types.NodeActive {
			c.Active++
		}
		counts[key] = c
	}
	for key, c := range counts {
		if err := k.OperatorNodeCounts.Set(ctx, collections.Join(key.operator, key.nodeType), c); err != nil {
			return err
		}
	}

	for _, nsc := range data.NodeStatusCounters {
		if err := k.NodeStatusCounters.Set(ctx, nsc.NodeAddress, nsc.Counter); err != nil {
			return err
		}
	}

	for _, gc := range data.GaslessCounters {
		if err := k.GaslessCounters.Set(ctx, collections.Join3(gc.Kind, gc.Signer, gc.Scope), gc.Counter); err != nil {
			return err
		}
	}

	return nil
}

// ExportGenesis exports the module's state to a genesis state. Derived
// structures are not exported; InitGenesis rebuilds them.
func (k *Keeper) ExportGenesis(ctx context.Context) *types.GenesisState {
	params, err := k.Params.Get(ctx)
	if err != nil {
		panic(err)
	}

	var nodes []types.Node
	if err := k.Nodes.Walk(ctx, nil, func(_ string, node types.Node) (bool, error) {
		nodes = append(nodes, node)
		return false, nil
	}); err != nil {
		panic(err)
	}

	var nodeTypes []types.NodeType
	if err := k.NodeTypes.Walk(ctx, nil, func(_ string, nt types.NodeType) (bool, error) {
		nodeTypes = append(nodeTypes, nt)
		return false, nil
	}); err != nil {
		panic(err)
	}

	var keys []types.ActivationKey
	if err := k.ActivationKeys.Walk(ctx, nil, func(_ string, key types.ActivationKey) (bool, error) {
		keys = append(keys, key)
		return false, nil
	}); err != nil {
		panic(err)
	}

	var statusCounters []types.NodeStatusCounter
	if err := k.NodeStatusCounters.Walk(ctx, nil, func(addr string, counter types.ActivityCounter) (bool, error) {
		statusCounters = append(statusCounters, types.NodeStatusCounter{NodeAddress: addr, Counter: counter})
		return false, nil
	}); err != nil {
		panic(err)
	}

	var gaslessCounters []types.GaslessCounter
	if err := k.GaslessCounters.Walk(ctx, nil, func(key collections.Triple[string, string, string], counter types.ActivityCounter) (bool, error) {
		gaslessCounters = append(gaslessCounters, types.GaslessCounter{Kind: key.K1(), Signer: key.K2(), Scope: key.K3(), Counter: counter})
		return false, nil
	}); err != nil {
		panic(err)
	}

	grants, err := k.Grants.Export(ctx)
	if err != nil {
		panic(err)
	}

	return &types.GenesisState{
		Params:             params,
		NodeTypes:          nodeTypes,
		Nodes:              nodes,
		ActivationKeys:     keys,
		NodeStatusCounters: statusCounters,
		GaslessCounters:    gaslessCounters,
		Grants:             grants,
	}
}
