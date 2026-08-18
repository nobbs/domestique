package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	testAccessEmail = "rider@example.com"
	testAssertion   = "header.claims.signature"
)

// recordingVerifier stands in for internal/cfaccess and records its use.
type recordingVerifier struct {
	err   error
	email string
	calls int
}

// Verify reports the configured outcome and counts the call.
func (v *recordingVerifier) Verify(_ context.Context, _ string) (string, error) {
	v.calls++
	if v.err != nil {
		return "", v.err
	}

	return v.email, nil
}

// loginRecordingOAuth captures the identity the gate passes downstream.
type loginRecordingOAuth struct {
	startLogin, completeLogin string
}

// Start records the caller identity the gate resolved.
func (o *loginRecordingOAuth) Start(_ context.Context, login, _ string) (string, error) {
	o.startLogin = login

	return "https://wahoo.example.test/authorize", nil
}

// Complete records the caller identity the gate resolved.
func (o *loginRecordingOAuth) Complete(_ context.Context, login, _, _ string) error {
	o.completeLogin = login

	return nil
}

// newAccessHandler builds a handler with the public Cloudflare path enabled.
func newAccessHandler(t *testing.T, verifier AccessVerifier, oauthService OAuth) *Handler {
	t.Helper()

	handler, err := New(
		&Options{
			TailnetUserLogin: "rider@example.ts.net",
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   verifier,
			AccessEmail:      testAccessEmail,
		},
		oauthService, &fakeState{}, &fakeSyncTrigger{accepted: true}, &fakeAssets{},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return handler
}

// assertionRequest is a request arriving on the public Cloudflare path: it
// carries a signed assertion and no Tailnet identity, because Tailscale Serve
// injects none for the tagged node cloudflared runs on.
func assertionRequest(t *testing.T, method, target string) *http.Request {
	t.Helper()

	request := httptest.NewRequestWithContext(t.Context(), method, target, http.NoBody)
	request.Header.Set(assertionHeader, testAssertion)

	return request
}

func TestGateAcceptsVerifiedAccessAssertion(t *testing.T) {
	verifier := &recordingVerifier{email: testAccessEmail}
	handler := newAccessHandler(t, verifier, &fakeOAuth{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, assertionRequest(t, http.MethodGet, "/v1/status"))

	if got, want := response.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if verifier.calls != 1 {
		t.Errorf("verifier calls = %d, want 1", verifier.calls)
	}
}

// The email claim's domain part is case-insensitive in practice, and an IdP may
// vary the case it asserts.
func TestGateAcceptsAssertionRegardlessOfEmailCase(t *testing.T) {
	handler := newAccessHandler(t, &recordingVerifier{email: "Rider@Example.COM"}, &fakeOAuth{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, assertionRequest(t, http.MethodGet, "/v1/status"))

	if got, want := response.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// A valid assertion for somebody else must not open the service. This is the
// check that matters if the Access policy is ever widened by accident.
func TestGateRejectsAssertionNamingAnotherEmail(t *testing.T) {
	handler := newAccessHandler(t, &recordingVerifier{email: "someone-else@example.com"}, &fakeOAuth{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, assertionRequest(t, http.MethodGet, "/v1/status"))

	if got, want := response.Code, http.StatusForbidden; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

func TestGateRejectsUnverifiableAssertion(t *testing.T) {
	handler := newAccessHandler(t, &recordingVerifier{err: errors.New("expired at 12:00 for key kid-7")}, &fakeOAuth{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, assertionRequest(t, http.MethodGet, "/v1/status"))

	if got, want := response.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	// The rejection must not describe the check the caller has to defeat.
	if body := response.Body.String(); strings.Contains(body, "kid-7") || strings.Contains(body, "expired") {
		t.Errorf("body = %q, want no verification detail", body)
	}
}

func TestGateRejectsMissingAssertionOnPublicPath(t *testing.T) {
	handler := newAccessHandler(t, &recordingVerifier{email: testAccessEmail}, &fakeOAuth{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/status", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// An assertion must not substitute for the Tailnet gate when the deployment has
// no public path configured at all.
func TestGateIgnoresAssertionWhenPublicPathDisabled(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, assertionRequest(t, http.MethodGet, "/v1/status"))

	if got, want := response.Code, http.StatusUnauthorized; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}

// Tailscale Serve strips client-supplied identity headers, so a present one is
// authoritative and settles the request without consulting Cloudflare. A
// forged assertion alongside it changes nothing.
func TestGatePrefersTailnetIdentityOverAssertion(t *testing.T) {
	verifier := &recordingVerifier{email: "someone-else@example.com"}
	handler := newAccessHandler(t, verifier, &fakeOAuth{})

	request := assertionRequest(t, http.MethodGet, "/v1/status")
	request.Header.Set(identityHeader, "rider@example.ts.net")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if verifier.calls != 0 {
		t.Errorf("verifier calls = %d, want 0", verifier.calls)
	}
}

// A wrong Tailnet identity is refused outright rather than falling through to
// the assertion, so the two paths cannot be combined into a way in.
func TestGateDoesNotFallBackToAssertionForWrongTailnetIdentity(t *testing.T) {
	verifier := &recordingVerifier{email: testAccessEmail}
	handler := newAccessHandler(t, verifier, &fakeOAuth{})

	request := assertionRequest(t, http.MethodGet, "/v1/status")
	request.Header.Set(identityHeader, "intruder@example.ts.net")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusForbidden; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
	if verifier.calls != 0 {
		t.Errorf("verifier calls = %d, want 0", verifier.calls)
	}
}

// The Wahoo OAuth redirect returns through Cloudflare, so a flow begun on the
// Tailnet finishes on the public path. Both paths must therefore hand the same
// principal to the OAuth service, or the caller-bound state never consumes.
func TestGateCanonicalisesIdentityAcrossBothPaths(t *testing.T) {
	oauthService := &loginRecordingOAuth{}
	handler := newAccessHandler(t, &recordingVerifier{email: testAccessEmail}, oauthService)

	tailnet := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/oauth/wahoo/start/rider-a", http.NoBody)
	tailnet.Header.Set(identityHeader, "rider@example.ts.net")
	handler.ServeHTTP(httptest.NewRecorder(), tailnet)

	public := assertionRequest(t, http.MethodGet, "/oauth/wahoo/callback?state=s&code=c")
	handler.ServeHTTP(httptest.NewRecorder(), public)

	if oauthService.startLogin != "rider@example.ts.net" {
		t.Errorf("start login = %q, want the configured principal", oauthService.startLogin)
	}
	if oauthService.completeLogin != oauthService.startLogin {
		t.Errorf("complete login = %q, want %q", oauthService.completeLogin, oauthService.startLogin)
	}
}

// A half-configured public path is a service that answers publicly without
// checking anything, so the constructor refuses it.
func TestNewRejectsHalfConfiguredAccess(t *testing.T) {
	cases := map[string]*Options{
		"verifier without email": {
			TailnetUserLogin: "rider@example.ts.net",
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
		},
		"email without verifier": {
			TailnetUserLogin: "rider@example.ts.net",
			TargetIDs:        []string{"rider-a"},
			TileStyleURL:     testTileStyleURL,
			AccessEmail:      testAccessEmail,
		},
	}

	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(options, &fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{}, &fakeAssets{}); err == nil {
				t.Fatal("expected rejection, got a handler")
			}
		})
	}
}

// Health stays reachable without any identity on either path, because Docker
// probes it over loopback.
func TestHealthRemainsUngatedWithPublicPathEnabled(t *testing.T) {
	handler := newAccessHandler(t, &recordingVerifier{email: testAccessEmail}, &fakeOAuth{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}
