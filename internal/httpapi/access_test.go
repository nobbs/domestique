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

// newAccessHandler builds a handler gated by the given verifier.
func newAccessHandler(t *testing.T, verifier AccessVerifier, oauthService OAuth) *Handler {
	t.Helper()

	handler, err := New(
		&Options{
			TargetIDs:      []string{"rider-a"},
			TileStyleURL:   testTileStyleURL,
			AccessVerifier: verifier,
			AccessEmail:    testAccessEmail,
		},
		oauthService, &fakeState{}, &fakeSyncTrigger{accepted: true}, &fakeAssets{},
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return handler
}

// assertionRequest is a request as it arrives from cloudflared: a signed
// assertion and no Tailnet identity, because Serve injects none for the tagged
// node cloudflared runs on.
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

// Tailscale-User-Login is not an identity here. Serve still fronts the
// listener and Tailnet members can still reach it directly, so honouring that
// header would be a second way in — and Cloudflare Tunnel forwards client
// headers verbatim, so it would be a forgeable one. Sending it must change
// nothing at all.
func TestGateIgnoresTailscaleIdentityHeader(t *testing.T) {
	tests := []struct {
		verifier *recordingVerifier
		name     string
		want     int
		wantCall int
		assert   bool
	}{
		{
			name:     "header alone is not identity",
			verifier: &recordingVerifier{email: testAccessEmail},
			assert:   false,
			want:     http.StatusUnauthorized,
			wantCall: 0,
		},
		{
			name:     "header does not skip verification",
			verifier: &recordingVerifier{email: testAccessEmail},
			assert:   true,
			want:     http.StatusOK,
			wantCall: 1,
		},
		{
			name:     "header does not rescue a bad assertion",
			verifier: &recordingVerifier{err: errors.New("bad signature")},
			assert:   true,
			want:     http.StatusUnauthorized,
			wantCall: 1,
		},
		{
			name:     "header does not override the asserted identity",
			verifier: &recordingVerifier{email: "someone-else@example.com"},
			assert:   true,
			want:     http.StatusForbidden,
			wantCall: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler := newAccessHandler(t, test.verifier, &fakeOAuth{})

			request := httptest.NewRequestWithContext(
				t.Context(), http.MethodGet, "/v1/status", http.NoBody)
			request.Header.Set("Tailscale-User-Login", testAccessEmail)
			if test.assert {
				request.Header.Set(assertionHeader, testAssertion)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if got := response.Code; got != test.want {
				t.Errorf("status = %d, want %d", got, test.want)
			}
			if got := test.verifier.calls; got != test.wantCall {
				t.Errorf("verifier calls = %d, want %d", got, test.wantCall)
			}
		})
	}
}

// The Wahoo OAuth flow binds its state to the caller identity, so a flow begun
// by one request must be consumable by the next. The gate hands downstream the
// configured address rather than the asserted spelling, which is what keeps
// that true when Access varies the case between assertions.
func TestGateResolvesEveryRequestToTheConfiguredPrincipal(t *testing.T) {
	oauthService := &loginRecordingOAuth{}
	verifier := &recordingVerifier{email: strings.ToUpper(testAccessEmail)}
	handler := newAccessHandler(t, verifier, oauthService)

	handler.ServeHTTP(httptest.NewRecorder(),
		assertionRequest(t, http.MethodGet, "/oauth/wahoo/start/rider-a"))
	handler.ServeHTTP(httptest.NewRecorder(),
		assertionRequest(t, http.MethodGet, "/oauth/wahoo/callback?state=s&code=c"))

	if oauthService.startLogin != testAccessEmail {
		t.Errorf("start login = %q, want %q", oauthService.startLogin, testAccessEmail)
	}
	if oauthService.completeLogin != oauthService.startLogin {
		t.Errorf("complete login = %q, want %q", oauthService.completeLogin, oauthService.startLogin)
	}
}

// Without a verifier the service has no gate at all, and without an allowed
// address it would admit anyone the team's IdP authenticates. Neither is a
// service worth starting.
func TestNewRequiresAccessConfiguration(t *testing.T) {
	cases := map[string]*Options{
		"no verifier": {
			TargetIDs:    []string{"rider-a"},
			TileStyleURL: testTileStyleURL,
			AccessEmail:  testAccessEmail,
		},
		"no email": {
			TargetIDs:      []string{"rider-a"},
			TileStyleURL:   testTileStyleURL,
			AccessVerifier: &recordingVerifier{email: testAccessEmail},
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

// Health stays reachable without any identity, because Docker probes it over
// loopback.
func TestHealthRemainsUngated(t *testing.T) {
	handler := newAccessHandler(t, &recordingVerifier{email: testAccessEmail}, &fakeOAuth{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if got, want := response.Code, http.StatusOK; got != want {
		t.Errorf("status = %d, want %d", got, want)
	}
}
