package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadyWhenLocalStateCarriesEveryConfiguredTarget(t *testing.T) {
	handler := newHandler(t, &fakeState{authorizations: map[string]string{
		"rider-a": "authorized",
		"rider-b": "needs_reauthorization",
	}}, "rider-a", "rider-b")

	response := probe(t, handler, "/readyz")

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, map[string]string{"status": "ready"}, decode(t, response))
}

// An unauthorised slot is a deployment waiting for a one-time browser visit, not
// a process that cannot run. Readiness must not hold a healthy container down
// until a human gets to it.
func TestReadyWhileATargetIsStillWaitingForItsAuthorisation(t *testing.T) {
	handler := newHandler(t, &fakeState{authorizations: map[string]string{
		"rider-a": "unauthorized",
	}}, "rider-a")

	assert.Equal(t, http.StatusOK, probe(t, handler, "/readyz").Code)
}

func TestUnreadyWhenLocalStateCannotBeRead(t *testing.T) {
	handler := newHandler(t, &fakeState{err: errors.New("state.db: disk I/O error")}, "rider-a")

	response := probe(t, handler, "/readyz")

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Equal(t, map[string]string{"status": "unready", "reason": "state_unreadable"}, decode(t, response))
	// The category travels; the failure detail stays in the process.
	assert.NotContains(t, response.Body.String(), "state.db")
}

func TestUnreadyWhenAConfiguredTargetHasNoStateRow(t *testing.T) {
	handler := newHandler(t, &fakeState{authorizations: map[string]string{
		"rider-a": "authorized",
	}}, "rider-a", "rider-b")

	response := probe(t, handler, "/readyz")

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	assert.Equal(t, map[string]string{"status": "unready", "reason": "state_incomplete"}, decode(t, response))
}

// The probe listener serves one path. It is not a second copy of the service's
// HTTP surface, and it does not answer for the liveness probe either.
func TestTheProbeListenerServesNothingElse(t *testing.T) {
	handler := newHandler(t, &fakeState{authorizations: map[string]string{"rider-a": "authorized"}}, "rider-a")

	for _, path := range []string{"/healthz", "/v1/status", "/", "/readyz/", "/routes/1/1"} {
		t.Run(path, func(t *testing.T) {
			assert.Equal(t, http.StatusNotFound, probe(t, handler, path).Code)
		})
	}
}

// Only a GET reads the probe. Anything else falls through to the catch-all
// rather than being answered, so a caller that tried to write here learns
// nothing about what this listener serves.
func TestTheProbeAnswersNothingButAGet(t *testing.T) {
	handler := newHandler(t, &fakeState{authorizations: map[string]string{"rider-a": "authorized"}}, "rider-a")

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/readyz", http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, map[string]string{"status": "not_found"}, decode(t, response))
}

func TestTheProbeResponseIsNeverCached(t *testing.T) {
	handler := newHandler(t, &fakeState{authorizations: map[string]string{"rider-a": "authorized"}}, "rider-a")

	response := probe(t, handler, "/readyz")

	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Equal(t, "nosniff", response.Header().Get("X-Content-Type-Options"))
}

// The probe reads state only. Nothing it is given could reach a provider, and a
// state read that outlives the probe's own bound is not allowed to hang it.
func TestTheProbeBoundsItsStateRead(t *testing.T) {
	state := &fakeState{authorizations: map[string]string{"rider-a": "authorized"}}
	handler := newHandler(t, state, "rider-a")

	probe(t, handler, "/readyz")

	require.NotNil(t, state.seen)
	deadline, ok := state.seen.Deadline()
	assert.True(t, ok, "the state read must carry a deadline")
	assert.False(t, deadline.IsZero())
}

func TestNewRefusesAnIncompleteWiring(t *testing.T) {
	_, err := New([]string{"rider-a"}, nil)
	require.Error(t, err)

	_, err = New(nil, &fakeState{})
	require.Error(t, err)

	_, err = New([]string{" "}, &fakeState{})
	require.Error(t, err)
}

func newHandler(t *testing.T, state State, targetIDs ...string) *Handler {
	t.Helper()
	handler, err := New(targetIDs, state)
	require.NoError(t, err)

	return handler
}

func probe(t *testing.T, handler *Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, http.NoBody)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}

func decode(t *testing.T, response *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var body map[string]string
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))

	return body
}

type fakeState struct {
	err            error
	seen           context.Context //nolint:containedctx // the test asserts the bound the handler applied
	authorizations map[string]string
}

func (s *fakeState) ForEachTarget(ctx context.Context, visit func(string, string) error) error {
	s.seen = ctx
	if s.err != nil {
		return s.err
	}
	for id, authorization := range s.authorizations {
		if err := visit(id, authorization); err != nil {
			return err
		}
	}

	return nil
}
