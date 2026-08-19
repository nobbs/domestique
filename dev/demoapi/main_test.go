package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/demo"
)

func demoTeam(t *testing.T) *team {
	t.Helper()

	local, err := newTeam(&config.CloudflareAccess{
		TeamDomain:     "demo",
		ApplicationAUD: "demo-application",
		AllowedEmail:   "rider@example.test",
	})
	require.NoError(t, err)

	return local
}

// The demo is only usable if the assertion it hands the dev server passes the
// production gate, so that round trip is the thing worth asserting: a signature
// the real verifier accepts, over the identity the configuration allows.
func TestMintedAssertionPassesTheProductionGate(t *testing.T) {
	local := demoTeam(t)

	assertion, err := local.mint()
	require.NoError(t, err)

	email, err := local.Verify(t.Context(), assertion)
	require.NoError(t, err)
	assert.Equal(t, "rider@example.test", email)
}

// A demo that could be talked into fetching keys from a real team would be a
// demo that reaches a provider, which is the one thing it must not do.
func TestKeySetTransportServesOnlyTheLocalTeam(t *testing.T) {
	local := demoTeam(t)

	_, err := local.Verify(t.Context(), "not.a.token")
	require.Error(t, err, "a malformed assertion must be rejected")

	other := demoTeam(t)
	other.issuer = "https://elsewhere.example.test"
	assertion, err := local.mint()
	require.NoError(t, err)

	_, err = other.Verify(t.Context(), assertion)
	require.Error(t, err, "a key set fetched from another origin must fail")
}

func TestSlotsForPairsConfiguredTargetsWithRequestedStates(t *testing.T) {
	targets := []config.Target{{ID: "rider-a"}, {ID: "rider-b"}}

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
	targets := []config.Target{{ID: "rider-a"}}

	_, err := slotsFor(targets, "current,failed")
	require.Error(t, err, "too many states must be rejected")

	_, err = slotsFor(targets, "lagging")
	require.Error(t, err, "an unknown state must be rejected")
}

func TestIssuerMatchesHowTheVerifierDerivesIt(t *testing.T) {
	tests := map[string]string{
		"demo":                       "https://demo.cloudflareaccess.com",
		"demo.cloudflareaccess.com":  "https://demo.cloudflareaccess.com",
		"https://demo.example.test/": "https://demo.example.test",
	}

	for domain, want := range tests {
		assert.Equal(t, want, issuerFor(domain), "issuerFor(%q)", domain)
	}
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
