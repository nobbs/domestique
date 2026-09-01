package main

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/demo"
)

// freeAddress reserves a loopback port and hands it back. The issuer needs its
// own address before it listens: the address is in the `iss` claim the SDK
// checks and in the certificate it verifies.
func freeAddress(t *testing.T) string {
	t.Helper()

	var config net.ListenConfig
	listener, err := config.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())

	return address
}

// The demo is only usable if what its issuer mints passes the production
// adapter, so that round trip is the thing worth asserting: a token the real
// SDK verifies, naming the subject the demo configuration allows.
func TestDemoIssuerSatisfiesTheProductionAdapter(t *testing.T) {
	tenant, err := newIssuer(freeAddress(t), "demo-client")
	require.NoError(t, err)
	stop, err := tenant.serve()
	require.NoError(t, err)
	t.Cleanup(stop)

	client, err := tenant.client("demo-secret", "https://127.0.0.1:9/auth/callback")
	require.NoError(t, err)

	const nonce = "demo-nonce"
	identity, err := client.Exchange(t.Context(),
		base64.RawURLEncoding.EncodeToString([]byte(nonce)), "demo-verifier", nonce)
	require.NoError(t, err)
	assert.Equal(t, demoSubject, identity.Subject)
}

// The nonce the authorization request carried has to come back in the ID
// token, or a replayed code would verify.
func TestDemoIssuerBindsTheNonceToTheCode(t *testing.T) {
	tenant, err := newIssuer(freeAddress(t), "demo-client")
	require.NoError(t, err)
	stop, err := tenant.serve()
	require.NoError(t, err)
	t.Cleanup(stop)

	client, err := tenant.client("demo-secret", "https://127.0.0.1:9/auth/callback")
	require.NoError(t, err)

	_, err = client.Exchange(t.Context(),
		base64.RawURLEncoding.EncodeToString([]byte("minted-for")), "demo-verifier", "asked-for")
	require.Error(t, err, "a token minted for another nonce must be refused")
}

func TestSlotsForPairsConfiguredTargetsWithRequestedStates(t *testing.T) {
	targets := []string{"rider-a", "rider-b"}

	slots, err := slotsFor(targets, "current, unauthorized")
	require.NoError(t, err)
	require.Len(t, slots, 2)
	assert.Equal(t, "rider-a", slots[0].ID, "the configured slots came back out of order")
	assert.Equal(t, "rider-b", slots[1].ID, "the configured slots came back out of order")
	assert.Equal(t, demo.SlotCurrent, slots[0].State)
	assert.Equal(t, demo.SlotUnauthorized, slots[1].State)
}

// A state list that does not line up with the configured slots would seed a
// target the served surface does not know about, or leave one unseeded.
func TestSlotsForRejectsAMismatchedRequest(t *testing.T) {
	targets := []string{"rider-a"}

	_, err := slotsFor(targets, "current,failed")
	require.Error(t, err, "too many states must be rejected")

	_, err = slotsFor(targets, "lagging")
	require.Error(t, err, "an unknown state must be rejected")
}

// A demo built without a bundle still serves the API, and says why the UI is not
// there. The alternative — a blank page from an index that is not embedded — is
// indistinguishable from an application that failed to start.
func TestUnbuiltAssetsExplainThemselves(t *testing.T) {
	for name, serve := range map[string]func(http.ResponseWriter, *http.Request){
		"index":  unbuiltAssets{}.Index,
		"static": unbuiltAssets{}.Static,
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			serve(recorder, httptest.NewRequestWithContext(
				t.Context(), http.MethodGet, "/", http.NoBody,
			))

			assert.Equal(t, http.StatusNotFound, recorder.Code)
			assert.Contains(t, recorder.Body.String(), "mise run ui-build",
				"the message names the command that builds a bundle")
		})
	}
}
