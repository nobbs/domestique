package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/auth0"
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

	_, _, _, _, _, err = provider.Exchange(t.Context(), "code", "verifier-1", "nonce-1")
	assert.Error(t, err, "Exchange() against an unroutable domain")
}
