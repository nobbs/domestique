package rider

import "slices"

// CredentialName identifies one credential held against a rider's own
// subject rather than against the deployment. It is the name the value is
// stored and encrypted under, so it is stable rather than cosmetic.
type CredentialName string

const (
	// CredentialZwiftEmail is the rider's own Zwift account email.
	CredentialZwiftEmail CredentialName = "zwift.email" //nolint:gosec // G101: a storage name, not a credential
	// CredentialZwiftPassword is the rider's own Zwift account password.
	CredentialZwiftPassword CredentialName = "zwift.password"
)

// Credential carries a rider's own credential without exposing it through
// formatting or JSON serialization, mirroring runtimeconfig.Secret.
type Credential struct {
	value []byte
}

// NewCredential wraps credential bytes. An empty value is a credential that
// is not set.
func NewCredential(value []byte) Credential {
	return Credential{value: slices.Clone(value)}
}

// Bytes returns a defensive copy of the credential.
func (c Credential) Bytes() []byte {
	return slices.Clone(c.value)
}

// IsSet reports whether a credential is stored, which is all any observable
// surface is ever told about one.
func (c Credential) IsSet() bool {
	return len(c.value) > 0
}

// String is what every formatting verb renders, so a credential interpolated
// into a message or handed to slog reads as this rather than as the bytes an
// unexported field would otherwise still print.
func (c Credential) String() string {
	return "[redacted]"
}

// GoString is the same for %#v.
func (c Credential) GoString() string {
	return "[redacted]"
}
