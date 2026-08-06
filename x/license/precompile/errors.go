package licenseprecompile

const (
	// ErrCallerIsNotIssuer is returned when an issueLicenses call originates from
	// an address that is not the issuer declared in the message.
	ErrCallerIsNotIssuer = "caller %s is not the declared issuer %s"
	// ErrCallerIsNotRevoker is returned when a revokeLicenses call originates from
	// an address that is not the revoker declared in the message.
	ErrCallerIsNotRevoker = "caller %s is not the declared revoker %s"
	// ErrCallerIsNotOwner is returned when an owner-gated call originates from a
	// non-owner address.
	ErrCallerIsNotOwner = "caller %s is not the module owner %s"
)
