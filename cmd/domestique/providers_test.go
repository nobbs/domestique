package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/auth0"
	"github.com/nobbs/domestique/internal/session"
)

// signInProvider is a thin forwarding adapter to *auth0.Client; these exercise
// that both methods forward rather than reimplement, against a client an
// unroutable domain guarantees never completes a real exchange.
func TestSignInProviderForwardsToTheAuth0Client(t *testing.T) {
	t.Parallel()

	client, err := auth0.New(&auth0.Options{
		Domain:       "127.0.0.1:1",
		ClientID:     "cid",
		ClientSecret: []byte("secret"),
		RedirectURL:  "https://app.example/callback",
	})
	require.NoError(t, err, "auth0.New()")
	provider := signInProvider{client: client}

	url, err := provider.AuthorizationURL(t.Context(), "state-1", "nonce-1", "verifier-1")
	require.NoError(t, err, "AuthorizationURL()")
	assert.Contains(t, url, "state=state-1", "AuthorizationURL() did not forward the state")

	_, err = provider.Exchange(t.Context(), "code", "verifier-1", "nonce-1")
	assert.Error(t, err, "Exchange() against an unroutable domain")
}

// The success path needs no client at all: exchangedIdentityFrom is exactly
// the mapping Exchange applies to whatever the client hands back, split out
// so a wrong or transposed field is caught here rather than only reachable
// through a live or fake token exchange.
func TestExchangedIdentityFromCopiesEveryField(t *testing.T) {
	t.Parallel()

	got := exchangedIdentityFrom(auth0.Identity{
		Subject: "github|123456",
		Email:   "rider@example.test",
		Name:    "Rider Example",
		Access:  true,
		Admin:   true,
	})

	assert.Equal(t, session.ExchangedIdentity{
		Subject: "github|123456",
		Email:   "rider@example.test",
		Name:    "Rider Example",
		Access:  true,
		Admin:   true,
	}, got)
}
