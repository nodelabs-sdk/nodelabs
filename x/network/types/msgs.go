package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	accesstypes "github.com/nodelabs-sdk/nodelabs/x/access/types"
)

var (
	_ sdk.Msg = &MsgCreateOperatorAccount{}
	_ sdk.Msg = &MsgCreateNodeType{}
	_ sdk.Msg = &MsgAuthorizeActivationKey{}
	_ sdk.Msg = &MsgDeauthorizeActivationKey{}
	_ sdk.Msg = &MsgActivateNode{}
	_ sdk.Msg = &MsgDeactivateNode{}
	_ sdk.Msg = &MsgUpdateNodeStatus{}
	_ sdk.Msg = &MsgUpdateParams{}
	_ sdk.Msg = &MsgTransferOwnership{}
	_ sdk.Msg = &MsgGrantAccess{}
	_ sdk.Msg = &MsgRevokeAccess{}
)

// GaslessMessages returns the msg type URLs this module admits with a zero
// fee, for composing an app's gasless allowlist. Consuming chains append
// their own module's gasless msgs (e.g. attestation msgs).
func GaslessMessages() []string {
	return []string{
		sdk.MsgTypeURL(&MsgAuthorizeActivationKey{}),
		sdk.MsgTypeURL(&MsgActivateNode{}),
		sdk.MsgTypeURL(&MsgDeactivateNode{}),
		sdk.MsgTypeURL(&MsgUpdateNodeStatus{}),
	}
}

// Every address field is validated in its canonical encoding, not merely as
// decodable bech32: these strings become store keys, and a non-canonical
// alias would mint a second identity for the same account (see
// ValidateCanonicalAddress).

func (msg *MsgCreateOperatorAccount) ValidateBasic() error {
	if err := ValidateCanonicalAddress("admin", msg.Admin); err != nil {
		return err
	}
	return ValidateCanonicalAddress("wallet", msg.Wallet)
}

// MsgCreateNodeType is deliberately absent from GaslessMessages: registering a
// node type is an administrative action by a granted address, not node
// software running unattended, so it pays gas like any other tx.
func (msg *MsgCreateNodeType) ValidateBasic() error {
	if err := ValidateCanonicalAddress("creator", msg.Creator); err != nil {
		return err
	}
	if msg.Id == "" {
		return ErrInvalidNodeType.Wrap("node type id must not be empty")
	}
	if msg.LicenseTypeId == "" {
		return ErrLicenseTypeNotFound.Wrap("license type id must not be empty")
	}
	return nil
}

func (msg *MsgAuthorizeActivationKey) ValidateBasic() error {
	if err := ValidateCanonicalAddress("operator", msg.Operator); err != nil {
		return err
	}
	if err := ValidateCanonicalAddress("activation", msg.ActivationAddress); err != nil {
		return err
	}
	if msg.ActivationAddress == msg.Operator {
		return ErrSelfAuthorization
	}
	return nil
}

func (msg *MsgDeauthorizeActivationKey) ValidateBasic() error {
	if err := ValidateCanonicalAddress("operator", msg.Operator); err != nil {
		return err
	}
	return ValidateCanonicalAddress("activation", msg.ActivationAddress)
}

func (msg *MsgActivateNode) ValidateBasic() error {
	if err := ValidateCanonicalAddress("activation", msg.ActivationAddress); err != nil {
		return err
	}
	if err := ValidateCanonicalAddress("operator", msg.Operator); err != nil {
		return err
	}
	if err := ValidateCanonicalAddress("node", msg.NodeAddress); err != nil {
		return err
	}
	if msg.NodeType == "" {
		return ErrInvalidNodeType.Wrap("node type must not be empty")
	}
	return nil
}

func (msg *MsgDeactivateNode) ValidateBasic() error {
	if err := ValidateCanonicalAddress("signer", msg.Signer); err != nil {
		return err
	}
	return ValidateCanonicalAddress("node", msg.NodeAddress)
}

func (msg *MsgUpdateNodeStatus) ValidateBasic() error {
	if err := ValidateCanonicalAddress("node", msg.NodeAddress); err != nil {
		return err
	}
	return msg.Payload.Validate()
}

// Validate bounds the payload fields. The chain treats the contents as
// opaque; only sizes are checked.
func (p NodeStatusPayload) Validate() error {
	for name, v := range map[string]string{
		"device": p.Device, "os": p.Os, "hostname": p.Hostname,
		"node_info": p.NodeInfo, "memory": p.Memory, "cpu": p.Cpu,
		"storage": p.Storage,
	} {
		if len(v) > MaxStatusFieldLen {
			return ErrInvalidStatusPayload.Wrapf("%s exceeds %d bytes", name, MaxStatusFieldLen)
		}
	}
	if len(p.Workloads) > MaxStatusWorkloads {
		return ErrInvalidStatusPayload.Wrapf("workloads length %d exceeds %d entries", len(p.Workloads), MaxStatusWorkloads)
	}
	for i, w := range p.Workloads {
		if len(w) > MaxStatusFieldLen {
			return ErrInvalidStatusPayload.Wrapf("workloads[%d] exceeds %d bytes", i, MaxStatusFieldLen)
		}
	}
	return nil
}

func (msg *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return ErrInvalidSigner.Wrapf("invalid authority address: %s", err)
	}
	return msg.Params.Validate()
}

func (msg *MsgTransferOwnership) ValidateBasic() error {
	if err := ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return err
	}
	// new_owner becomes Params.Owner, which every owner check compares by
	// string equality — a non-canonical alias there would be an owner nobody
	// can authenticate as.
	return ValidateCanonicalAddress("new owner", msg.NewOwner)
}

func (msg *MsgGrantAccess) ValidateBasic() error {
	if err := ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return err
	}
	// The grantee is a state key. A non-canonical alias would be a second
	// identity for the same account: exercisable by its holder, but invisible
	// to a revoke or an audit query using the canonical form.
	if err := ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return err
	}
	return accesstypes.ValidateGrantEntries(msg.Grants)
}

func (msg *MsgRevokeAccess) ValidateBasic() error {
	if err := ValidateCanonicalAddress("owner", msg.Owner); err != nil {
		return err
	}
	if err := ValidateCanonicalAddress("grantee", msg.Grantee); err != nil {
		return err
	}
	return accesstypes.ValidateRevokePairs(msg.Actions)
}
