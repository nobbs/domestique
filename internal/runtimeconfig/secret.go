package runtimeconfig

import (
	"fmt"
	"slices"

	"github.com/nobbs/domestique/internal/route"
)

// SecretName identifies one credential the service holds. It is the name the
// value is stored and encrypted under, so it is stable rather than cosmetic.
type SecretName string

const (
	// SecretWahooClientSecret is the OAuth application's client secret.
	SecretWahooClientSecret SecretName = "wahoo.client_secret"
	// SecretVeloPlannerEmail is the VeloPlanner account's email address.
	SecretVeloPlannerEmail SecretName = "veloplanner.email" //nolint:gosec // G101: the name a credential is stored under, not one
	// SecretVeloPlannerPassword is the VeloPlanner account's password.
	SecretVeloPlannerPassword SecretName = "veloplanner.password"
	// SecretKomootEmail is the Komoot account's email address.
	SecretKomootEmail SecretName = "komoot.email" //nolint:gosec // G101: the name a credential is stored under, not one
	// SecretKomootPassword is the Komoot account's password.
	SecretKomootPassword SecretName = "komoot.password"
	// SecretPushoverApplicationToken is the Pushover application's token.
	SecretPushoverApplicationToken SecretName = "notifications.pushover.application_token"
	// SecretPushoverUserKey is the Pushover recipient the messages are sent to.
	SecretPushoverUserKey SecretName = "notifications.pushover.user_key"
)

// SecretNames lists every credential the service holds, in the order a settings
// page offers them. A name outside this list is refused on the way in, so a
// stored secret always has something that reads it.
func SecretNames() []SecretName {
	return []SecretName{
		SecretWahooClientSecret,
		SecretVeloPlannerEmail,
		SecretVeloPlannerPassword,
		SecretKomootEmail,
		SecretKomootPassword,
		SecretPushoverApplicationToken,
		SecretPushoverUserKey,
	}
}

// ParseSecretName reads a submitted name, refusing one no part of the service
// reads: an unknown name is a page asking for a credential to be stored where
// nothing would ever look for it.
func ParseSecretName(name string) (SecretName, error) {
	if slices.Contains(SecretNames(), SecretName(name)) {
		return SecretName(name), nil
	}

	return "", fmt.Errorf("unknown secret %q", name)
}

// SourceSecretNames returns the account credentials one provider's library is
// read with, and whether this package knows the provider at all.
func SourceSecretNames(provider route.Provider) (email, password SecretName, known bool) {
	switch provider {
	case route.ProviderVeloPlanner:
		return SecretVeloPlannerEmail, SecretVeloPlannerPassword, true
	case route.ProviderKomoot:
		return SecretKomootEmail, SecretKomootPassword, true
	}

	return "", "", false
}

// Secret carries a credential without exposing it through formatting or JSON
// serialization.
type Secret struct {
	value []byte
}

// NewSecret wraps credential bytes. An empty value is a secret that is not set.
func NewSecret(value []byte) Secret {
	return Secret{value: slices.Clone(value)}
}

// Bytes returns a defensive copy of the credential.
func (s Secret) Bytes() []byte {
	return slices.Clone(s.value)
}

// IsSet reports whether a credential is stored, which is all any observable
// surface is ever told about one.
func (s Secret) IsSet() bool {
	return len(s.value) > 0
}

// String is what every formatting verb but %#v renders, so a credential
// interpolated into a message or handed to slog reads as this rather than as
// the bytes an unexported field would otherwise still print.
func (s Secret) String() string {
	return "[redacted]"
}
