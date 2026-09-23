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
	// CredentialWahooEmail is the rider's own Wahoo account email, used only for
	// the device sign-in that sets each route's provider_id.
	CredentialWahooEmail CredentialName = "wahoo.email"
	// CredentialWahooPassword is the rider's own Wahoo account password.
	CredentialWahooPassword CredentialName = "wahoo.password"
	// CredentialWahooSession is the device session token that sign-in issued.
	// The API can neither refresh nor end one, so it is kept and reused.
	CredentialWahooSession CredentialName = "wahoo.session"
	// CredentialWahooRefused marks the rider's Wahoo email and password as
	// refused by the last sign-in; it holds no secret and is cleared by a save.
	CredentialWahooRefused CredentialName = "wahoo.refused"
)

// WahooCredentialNames are every name a Wahoo save or removal resets, so new
// credentials never reuse a session or a refusal the old ones earned.
func WahooCredentialNames() []CredentialName {
	return []CredentialName{
		CredentialWahooEmail, CredentialWahooPassword, CredentialWahooSession, CredentialWahooRefused,
	}
}

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
