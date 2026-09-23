package rider

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
)

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
	// CredentialWahooSession is the device session token that sign-in issued,
	// bound to the pair it was issued for; see NewWahooSession.
	CredentialWahooSession CredentialName = "wahoo.session"
	// CredentialWahooRefused marks the pair the last sign-in refused; see
	// NewWahooRefusal.
	CredentialWahooRefused CredentialName = "wahoo.refused"
)

// WahooCredentialNames are every name a Wahoo save or removal resets, so new
// credentials never reuse a session or a refusal the old ones earned.
func WahooCredentialNames() []CredentialName {
	return []CredentialName{
		CredentialWahooEmail, CredentialWahooPassword, CredentialWahooSession, CredentialWahooRefused,
	}
}

// NewWahooSession binds a device session token to the Wahoo pair it was
// issued for, so a session stored after the rider changed the pair is ignored.
func NewWahooSession(credentials map[CredentialName]Credential, token string) Credential {
	return NewCredential([]byte(wahooPairFingerprint(credentials) + ":" + token))
}

// NewWahooRefusal marks the current Wahoo pair refused, on the same terms.
func NewWahooRefusal(credentials map[CredentialName]Credential) Credential {
	return NewCredential([]byte(wahooPairFingerprint(credentials)))
}

// WahooSession returns the stored device session token when it was issued for
// the current Wahoo pair.
func WahooSession(credentials map[CredentialName]Credential) (string, bool) {
	fingerprint, token, found := strings.Cut(string(credentials[CredentialWahooSession].value), ":")
	if !found || token == "" || fingerprint != wahooPairFingerprint(credentials) {
		return "", false
	}

	return token, true
}

// WahooRefused reports whether the last sign-in refused the current Wahoo pair.
func WahooRefused(credentials map[CredentialName]Credential) bool {
	refusal := credentials[CredentialWahooRefused]

	return refusal.IsSet() && bytes.Equal(refusal.value, []byte(wahooPairFingerprint(credentials)))
}

func wahooPairFingerprint(credentials map[CredentialName]Credential) string {
	digest := sha256.New()
	digest.Write(credentials[CredentialWahooEmail].value)
	digest.Write([]byte{0})
	digest.Write(credentials[CredentialWahooPassword].value)

	return hex.EncodeToString(digest.Sum(nil))
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
