package main

import (
	"crypto/tls"
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	tenant, err := newIssuer(freeAddress(t), "demo-client", "http://127.0.0.1:9/auth/callback")
	require.NoError(t, err)
	stop, err := tenant.serve()
	require.NoError(t, err)
	t.Cleanup(stop)

	client, err := tenant.client([]byte("demo-secret"), "https://127.0.0.1:9/auth/callback")
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
	tenant, err := newIssuer(freeAddress(t), "demo-client", "http://127.0.0.1:9/auth/callback")
	require.NoError(t, err)
	stop, err := tenant.serve()
	require.NoError(t, err)
	t.Cleanup(stop)

	client, err := tenant.client([]byte("demo-secret"), "https://127.0.0.1:9/auth/callback")
	require.NoError(t, err)

	_, err = client.Exchange(t.Context(),
		base64.RawURLEncoding.EncodeToString([]byte("minted-for")), "demo-verifier", "asked-for")
	require.Error(t, err, "a token minted for another nonce must be refused")
}

// The real service derives redirect_uri from browser_origin_url, which the
// demo config keeps deliberately unroutable. The issuer must send the browser
// back to its own callback address instead, carrying state and code intact.
func TestDemoIssuerAuthorizeRedirectsToItsOwnCallback(t *testing.T) {
	tenant, err := newIssuer(freeAddress(t), "demo-client", "http://127.0.0.1:9999/auth/callback")
	require.NoError(t, err)
	stop, err := tenant.serve()
	require.NoError(t, err)
	t.Cleanup(stop)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{RootCAs: tenant.roots}},
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+tenant.address+
		"/authorize?redirect_uri=https://127.0.0.1:9/auth/callback&state=demo-state&nonce=demo-nonce", http.NoBody)
	require.NoError(t, err)
	response, err := client.Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, response.Body.Close()) })

	require.Equal(t, http.StatusFound, response.StatusCode)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:9999", location.Host, "the browser goes to the demo's own callback, not redirect_uri")
	assert.Equal(t, "/auth/callback", location.Path)
	assert.Equal(t, "demo-state", location.Query().Get("state"))
	assert.Equal(t, base64.RawURLEncoding.EncodeToString([]byte("demo-nonce")), location.Query().Get("code"))
}

func TestNewIssuerRefusesACallbackURLThatIsNotAbsolute(t *testing.T) {
	for name, callbackURL := range map[string]string{
		"empty":     "",
		"relative":  "/auth/callback",
		"no scheme": "127.0.0.1:9999/auth/callback",
		"not a url": "http://[::1/auth/callback",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := newIssuer(freeAddress(t), "demo-client", callbackURL)
			require.Error(t, err)
			assert.NotContains(t, err.Error(), "<nil>", "a validation failure reported a nil cause")
		})
	}
}

func TestDefaultCallbackURLUsesLoopback(t *testing.T) {
	value, err := defaultCallbackURL(":8082")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:8082/auth/callback", value)
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
