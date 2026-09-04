package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func captureLog(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buffer bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buffer, &slog.HandlerOptions{Level: level})))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return &buffer
}

func serveLogged(t *testing.T, target string, status int, took time.Duration) {
	t.Helper()
	clock := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	handler := &Handler{now: func() time.Time {
		now := clock
		clock = clock.Add(took)

		return now
	}}
	recorder := httptest.NewRecorder()
	handler.logged(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(status)
	})).ServeHTTP(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody))
	require.Equal(t, status, recorder.Code)
}

func TestLoggedReportsFailuresWithoutRouteIDsOrQueries(t *testing.T) {
	output := captureLog(t, slog.LevelInfo)

	serveLogged(t, "/v1/routes/abc123/geometry?token=secret", http.StatusInternalServerError, 0)

	var line map[string]any
	require.NoError(t, json.Unmarshal(output.Bytes(), &line))
	assert.Equal(t, "ERROR", line["level"])
	assert.Equal(t, "request failed", line["msg"])
	assert.Equal(t, "/v1/routes", line["path"])
	assert.InDelta(t, http.StatusInternalServerError, line["status"], 0)
	assert.NotContains(t, output.String(), "abc123")
	assert.NotContains(t, output.String(), "secret")
}

func TestLoggedReportsSlowAnswers(t *testing.T) {
	output := captureLog(t, slog.LevelInfo)

	serveLogged(t, "/v1/status", http.StatusOK, slowRequest+time.Millisecond)

	assert.Contains(t, output.String(), `"msg":"request slow"`)
	assert.Contains(t, output.String(), `"duration_ms":2001`)
}

func TestLoggedKeepsRefusalsAtDebug(t *testing.T) {
	output := captureLog(t, slog.LevelInfo)
	serveLogged(t, "/v1/status", http.StatusUnauthorized, 0)
	assert.Empty(t, output.String(), "a refusal at Info")

	output = captureLog(t, slog.LevelDebug)
	serveLogged(t, "/v1/status", http.StatusForbidden, 0)
	assert.Contains(t, output.String(), `"msg":"request refused"`)
}

func TestLoggedIsSilentOnSuccess(t *testing.T) {
	output := captureLog(t, slog.LevelDebug)

	serveLogged(t, "/healthz", http.StatusOK, time.Millisecond)

	assert.Empty(t, output.String())
}

func TestStatusRecorderKeepsTheFirstStatusLikeNetHTTP(t *testing.T) {
	recorder := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	recorder.WriteHeader(http.StatusBadGateway)
	recorder.WriteHeader(http.StatusOK)
	assert.Equal(t, http.StatusBadGateway, recorder.status)
}

func TestStatusRecorderUnwrapsForTheResponseController(t *testing.T) {
	recorder := httptest.NewRecorder()
	assert.Same(t, recorder, (&statusRecorder{ResponseWriter: recorder}).Unwrap())
}

func TestPathClass(t *testing.T) {
	assert.Equal(t, "/", pathClass("/"))
	assert.Equal(t, "/healthz", pathClass("/healthz"))
	assert.Equal(t, "/auth/callback", pathClass("/auth/callback"))
	assert.Equal(t, "/v1/routes", pathClass("/v1/routes/abc/geometry"))
}
