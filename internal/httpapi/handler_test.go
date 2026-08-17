package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandlerGatesStateAndKeepsHealthLocal(t *testing.T) {
	handler := newTestHandler(t)
	health := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/healthz", http.NoBody)
	healthResponse := httptest.NewRecorder()
	handler.ServeHTTP(healthResponse, health)
	if got, want := healthResponse.Code, http.StatusOK; got != want {
		t.Errorf("health status = %d, want %d", got, want)
	}

	status := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/status", http.NoBody)
	statusResponse := httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	if got, want := statusResponse.Code, http.StatusUnauthorized; got != want {
		t.Errorf("unauthenticated status = %d, want %d", got, want)
	}

	status.Header.Set(identityHeader, "rider@example.ts.net")
	statusResponse = httptest.NewRecorder()
	handler.ServeHTTP(statusResponse, status)
	if got, want := statusResponse.Code, http.StatusOK; got != want {
		t.Errorf("authenticated status = %d, want %d", got, want)
	}
	if body := statusResponse.Body.String(); strings.Contains(body, "private-token") || !strings.Contains(body, "authorised") {
		t.Errorf("status body = %q, want safe authorization state", body)
	}
}

func TestHandlerRunsCallerBoundOAuthFlow(t *testing.T) {
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := newHandler(t, oauthService, &fakeState{})
	start := authenticatedRequest(http.MethodGet, "/oauth/wahoo/start/rider-a")
	startResponse := httptest.NewRecorder()
	handler.ServeHTTP(startResponse, start)
	if got, want := startResponse.Code, http.StatusFound; got != want {
		t.Errorf("start status = %d, want %d", got, want)
	}
	if got, want := oauthService.targetID, "rider-a"; got != want {
		t.Errorf("oauth target = %q, want %q", got, want)
	}

	callback := authenticatedRequest(http.MethodGet, "/oauth/wahoo/callback?state=state&code=code")
	callbackResponse := httptest.NewRecorder()
	handler.ServeHTTP(callbackResponse, callback)
	if got, want := callbackResponse.Code, http.StatusSeeOther; got != want {
		t.Errorf("callback status = %d, want %d", got, want)
	}
	if got, want := callbackResponse.Header().Get("Location"), "/v1/status"; got != want {
		t.Errorf("callback location = %q, want %q", got, want)
	}
}

func TestHandlerAcceptsManualSync(t *testing.T) {
	trigger := &fakeSyncTrigger{accepted: true}
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, trigger)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	if got, want := response.Code, http.StatusAccepted; got != want {
		t.Errorf("sync status = %d, want %d", got, want)
	}
	if got, want := trigger.calls, 1; got != want {
		t.Errorf("trigger calls = %d, want %d", got, want)
	}
}

func TestHandlerRejectsOverlappingManualSync(t *testing.T) {
	handler := newHandlerWithTrigger(t, &fakeOAuth{}, &fakeState{}, &fakeSyncTrigger{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodPost, "/v1/sync"))
	if got, want := response.Code, http.StatusConflict; got != want {
		t.Errorf("sync status = %d, want %d", got, want)
	}
}

func TestHandlerRejectsInactiveTarget(t *testing.T) {
	oauthService := &fakeOAuth{location: "https://wahoo.example.test/oauth/authorize"}
	handler := newHandler(t, oauthService, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/oauth/wahoo/start/rider-b"))
	if got, want := response.Code, http.StatusNotFound; got != want {
		t.Errorf("start status = %d, want %d", got, want)
	}
	if got := oauthService.targetID; got != "" {
		t.Errorf("OAuth start target = %q, want no OAuth request", got)
	}
}

func TestHandlerHidesOAuthFailure(t *testing.T) {
	handler := newHandler(t, &fakeOAuth{completeErr: errors.New("private-token")}, &fakeState{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/oauth/wahoo/callback?state=state&code=code"))
	if got, want := response.Code, http.StatusBadRequest; got != want {
		t.Errorf("callback status = %d, want %d", got, want)
	}
	if body := response.Body.String(); strings.Contains(body, "private-token") {
		t.Errorf("callback body exposed upstream error: %q", body)
	}
}

func newTestHandler(t *testing.T) *Handler { return newHandler(t, &fakeOAuth{}, &fakeState{}) }
func newHandler(t *testing.T, oauthService OAuth, state State) *Handler {
	return newHandlerWithTrigger(t, oauthService, state, &fakeSyncTrigger{accepted: true})
}

func newHandlerWithTrigger(t *testing.T, oauthService OAuth, state State, syncTrigger SyncTrigger) *Handler {
	t.Helper()
	handler, err := New("rider@example.ts.net", []string{"rider-a"}, oauthService, state, syncTrigger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return handler
}
func authenticatedRequest(method, target string) *http.Request {
	request := httptest.NewRequestWithContext(context.Background(), method, target, http.NoBody)
	request.Header.Set(identityHeader, "rider@example.ts.net")
	return request
}

type fakeOAuth struct {
	completeErr        error
	location, targetID string
}

type fakeSyncTrigger struct {
	accepted bool
	calls    int
}

func (t *fakeSyncTrigger) Trigger() bool {
	t.calls++

	return t.accepted
}

func (o *fakeOAuth) Start(_ context.Context, _, targetID string) (string, error) {
	o.targetID = targetID
	return o.location, nil
}
func (o *fakeOAuth) Complete(context.Context, string, string, string) error { return o.completeErr }

type fakeState struct{}

func (*fakeState) ForEachTarget(_ context.Context, visit func(string, string) error) error {
	return visit("rider-a", "authorised")
}
func (*fakeState) ForEachSourceStage(context.Context, func(int64, int, string, string) error) error {
	return nil
}

//nolint:gocritic // This fake conforms to the persistence-free state boundary.
func (*fakeState) LastSyncRun(context.Context) (time.Time, string, string, int, int, int, int, bool, error) {
	return time.Time{}, "", "", 0, 0, 0, 0, false, nil
}
