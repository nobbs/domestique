package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWorldMaps answers with what a test gave it, and counts what was asked.
type fakeWorldMaps struct {
	err      error
	data     []byte
	asked    []int64
	notFound bool
}

func (m *fakeWorldMaps) Image(
	_ context.Context, worldID int64,
) (data []byte, contentType string, found bool, err error) {
	m.asked = append(m.asked, worldID)
	if m.err != nil {
		return nil, "", false, m.err
	}
	if m.notFound {
		return nil, "", false, nil
	}

	return m.data, "image/png", true, nil
}

func worldMapHandler(t *testing.T, maps ZwiftWorldMaps) *Handler {
	t.Helper()
	handler := newTestHandler(t)
	handler.zwiftWorldMaps = maps

	return handler
}

func TestGetZwiftWorldMapServesTheArtwork(t *testing.T) {
	maps := &fakeWorldMaps{data: []byte("png-bytes")}
	handler := worldMapHandler(t, maps)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/zwift/worlds/9/map"))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "png-bytes", response.Body.String())
	assert.Equal(t, "image/png", response.Header().Get("Content-Type"))
	assert.Equal(t, cacheZwiftWorldMap, response.Header().Get("Cache-Control"))
	assert.NotEmpty(t, response.Header().Get("ETag"), "the bytes name themselves")
	assert.Equal(t, []int64{9}, maps.asked)
}

func TestGetZwiftWorldMapAnswersACurrentCopyWithoutTheBytes(t *testing.T) {
	handler := worldMapHandler(t, &fakeWorldMaps{data: []byte("png-bytes")})

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, authenticatedRequest(http.MethodGet, "/v1/zwift/worlds/9/map"))
	require.Equal(t, http.StatusOK, first.Code)

	conditional := authenticatedRequest(http.MethodGet, "/v1/zwift/worlds/9/map")
	conditional.Header.Set("If-None-Match", first.Header().Get("ETag"))
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, conditional)

	assert.Equal(t, http.StatusNotModified, second.Code)
	assert.Empty(t, second.Body.String())
}

func TestGetZwiftWorldMapIsNotFoundForAWorldThatDoesNotExist(t *testing.T) {
	handler := worldMapHandler(t, &fakeWorldMaps{notFound: true})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/zwift/worlds/12/map"))

	assert.Equal(t, http.StatusNotFound, response.Code)
}

// A build with no Zwift source wired serves no artwork rather than panicking on
// a port nobody supplied.
func TestGetZwiftWorldMapIsNotFoundWithoutTheRelay(t *testing.T) {
	handler := newTestHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/zwift/worlds/9/map"))

	assert.Equal(t, http.StatusNotFound, response.Code)
}

func TestGetZwiftWorldMapReportsAnUnreachableCDN(t *testing.T) {
	handler := worldMapHandler(t, &fakeWorldMaps{err: errors.New("cdn refused")})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/zwift/worlds/9/map"))

	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "cdn refused", "an upstream's own words never reach a caller")
}

func TestGetZwiftWorldMapNeedsASession(t *testing.T) {
	handler := worldMapHandler(t, &fakeWorldMaps{data: []byte("png-bytes")})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/v1/zwift/worlds/9/map", http.NoBody,
	))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}
