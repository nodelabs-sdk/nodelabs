package types

import "cosmossdk.io/errors"

// Codespace is the error codespace for grant-store failures. It is shared by
// every module that embeds a grant store rather than being registered per
// module, so a client can match on the code without knowing which module
// raised it; the error message always names the module.
const Codespace = "access"

var (
	ErrInvalidAction = errors.Register(Codespace, 1100, "invalid action")
	ErrInvalidScope  = errors.Register(Codespace, 1101, "invalid scope")
)
