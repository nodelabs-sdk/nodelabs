package types

const (
	EventTypeCreateOperatorAccount    = "create_operator_account"
	EventTypeCreateNodeType           = "create_node_type"
	EventTypeAuthorizeActivationKey   = "authorize_activation_key"
	EventTypeDeauthorizeActivationKey = "deauthorize_activation_key"
	EventTypeActivateNode             = "activate_node"
	EventTypeDeactivateNode           = "deactivate_node"
	EventTypeNodeStatus               = "node_status"

	AttributeKeyOperator          = "operator"
	AttributeKeyWallet            = "wallet"
	AttributeKeyActivationAddress = "activation_address"
	AttributeKeyNodeAddress       = "node_address"
	AttributeKeyNodeType          = "node_type"
	AttributeKeyLicenseTypeID     = "license_type_id"
	AttributeKeySigner            = "signer"
	AttributeKeyFee               = "fee"

	// node_status payload attributes.
	AttributeKeyDevice    = "device"
	AttributeKeyOS        = "os"
	AttributeKeyHostname  = "hostname"
	AttributeKeyNodeInfo  = "node_info"
	AttributeKeyWorkloads = "workloads"
	AttributeKeyMemory    = "memory"
	AttributeKeyCPU       = "cpu"
	AttributeKeyStorage   = "storage"
)
