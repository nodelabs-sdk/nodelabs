package types

import (
	"fmt"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:             DefaultParams(),
		NodeTypes:          []NodeType{},
		Nodes:              []Node{},
		ActivationKeys:     []ActivationKey{},
		NodeStatusCounters: []NodeStatusCounter{},
		Grants:             []accesstypes.Grant{},
	}
}

// Validate checks the genesis state's structural invariants. Only invariants
// the msg handlers maintain unconditionally are enforced; param-dependent
// limits (e.g. max_activation_keys) are not, because params may be lowered by
// governance after state accrued under higher ones and export/import must
// round-trip.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}

	if err := accesstypes.ValidateGrants(gs.Grants); err != nil {
		return err
	}
	// This module does not scope its actions, so a grant may only name an
	// action in the vocabulary and must carry the empty scope.
	spec := Spec()
	for i, g := range gs.Grants {
		if !spec.HasAction(g.Action) {
			return fmt.Errorf("grant %d: action %q is not part of this module's vocabulary", i, g.Action)
		}
		if g.Scope != "" {
			return fmt.Errorf("grant %d: action %q is module-wide: scope must be empty, got %q", i, g.Action, g.Scope)
		}
	}

	// Node types are validated first: the node loop below cross-references
	// them, which is only meaningful once the set itself is known good.
	//
	// license_type_id is checked for shape only. Whether it names a real
	// license type is a fact about x/license state and is invisible from here;
	// the msg handler is what enforces it at registration time.
	nodeTypeIDs := make(map[string]struct{}, len(gs.NodeTypes))
	boundLicenseTypes := make(map[string]string, len(gs.NodeTypes))
	for _, nt := range gs.NodeTypes {
		if _, dup := nodeTypeIDs[nt.Id]; dup {
			return fmt.Errorf("duplicate node type %s", nt.Id)
		}
		if nt.Id == "" {
			return fmt.Errorf("node type id must not be empty")
		}
		if nt.LicenseTypeId == "" {
			return fmt.Errorf("node type %s: license_type_id must not be empty", nt.Id)
		}
		// The binding is one-to-one. Two node types sharing a license type
		// would collide in the derived NodeTypeByLicenseType map, so one would
		// silently win on import.
		if other, dup := boundLicenseTypes[nt.LicenseTypeId]; dup {
			return fmt.Errorf("node types %s and %s are both bound to license type %s", other, nt.Id, nt.LicenseTypeId)
		}
		boundLicenseTypes[nt.LicenseTypeId] = nt.Id
		nodeTypeIDs[nt.Id] = struct{}{}
	}

	keyOperators := make(map[string]string, len(gs.ActivationKeys))
	for _, key := range gs.ActivationKeys {
		if _, dup := keyOperators[key.Address]; dup {
			return fmt.Errorf("duplicate activation key %s", key.Address)
		}
		// Canonical form, not merely decodable: these strings are the store
		// keys, so two encodings of one account would be two identities.
		if err := ValidateCanonicalAddress("activation", key.Address); err != nil {
			return fmt.Errorf("activation key %s: %w", key.Address, err)
		}
		if err := ValidateCanonicalAddress("operator", key.Operator); err != nil {
			return fmt.Errorf("activation key %s: %w", key.Address, err)
		}
		if key.Address == key.Operator {
			return fmt.Errorf("activation key %s: address equals its operator", key.Address)
		}
		if key.Status != KeyActive && key.Status != KeyDisabled {
			return fmt.Errorf("activation key %s: invalid status %q", key.Address, key.Status.String())
		}
		if key.CreatedAt.IsZero() {
			return fmt.Errorf("activation key %s: created_at must be set", key.Address)
		}
		keyOperators[key.Address] = key.Operator
	}

	nodeAddrs := make(map[string]struct{}, len(gs.Nodes))
	for _, node := range gs.Nodes {
		if _, dup := nodeAddrs[node.Address]; dup {
			return fmt.Errorf("duplicate node %s", node.Address)
		}
		if err := ValidateCanonicalAddress("node", node.Address); err != nil {
			return fmt.Errorf("node %s: %w", node.Address, err)
		}
		if err := ValidateCanonicalAddress("operator", node.Operator); err != nil {
			return fmt.Errorf("node %s: %w", node.Address, err)
		}
		if node.Status != NodeActive && node.Status != NodeDeactivated {
			return fmt.Errorf("node %s: invalid status %q", node.Address, node.Status.String())
		}
		// Referential, not merely non-empty: activation now resolves the type
		// through the registry, and node type records are never removed, so
		// every node's type is registered by construction.
		if _, exists := nodeTypeIDs[node.Type]; !exists {
			return fmt.Errorf("node %s: type %q is not a listed node type", node.Address, node.Type)
		}
		if node.LastActiveTime.IsZero() {
			return fmt.Errorf("node %s: last_active_time must be set", node.Address)
		}
		keyOperator, found := keyOperators[node.ActivatedBy]
		if !found {
			return fmt.Errorf("node %s: activated_by %q is not a listed activation key", node.Address, node.ActivatedBy)
		}
		if keyOperator != node.Operator {
			return fmt.Errorf("node %s: activated_by %q is bound to operator %s, not %s", node.Address, node.ActivatedBy, keyOperator, node.Operator)
		}
		nodeAddrs[node.Address] = struct{}{}
	}

	counterNodes := make(map[string]struct{}, len(gs.NodeStatusCounters))
	for _, nsc := range gs.NodeStatusCounters {
		if _, dup := counterNodes[nsc.NodeAddress]; dup {
			return fmt.Errorf("duplicate node status counter for %s", nsc.NodeAddress)
		}
		if _, exists := nodeAddrs[nsc.NodeAddress]; !exists {
			return fmt.Errorf("node status counter references unknown node %s", nsc.NodeAddress)
		}
		if nsc.Counter.LatestTime.IsZero() {
			return fmt.Errorf("node status counter for %s: latest_time must be set", nsc.NodeAddress)
		}
		counterNodes[nsc.NodeAddress] = struct{}{}
	}

	return nil
}
