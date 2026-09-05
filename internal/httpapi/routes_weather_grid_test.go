package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeatherGridLatestRequiresASession(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/v1/weather-grid/latest", http.NoBody,
	))

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestWeatherGridLatestRelaysTheUpstreamBodyAndHeaders(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		LatestFunc: func(context.Context) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/json")
			recorder.Header().Set("ETag", `"abc123"`)
			recorder.WriteHeader(http.StatusOK)
			_, err := recorder.WriteString(`{"reference_time":"2026-09-05T12:00:00Z"}`)
			require.NoError(t, err)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather-grid/latest"))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
	assert.Equal(t, `"abc123"`, response.Header().Get("ETag"))
	assert.JSONEq(t, `{"reference_time":"2026-09-05T12:00:00Z"}`, response.Body.String())
}

func TestWeatherGridLatestReturnsBadGatewayOnProviderFailure(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		LatestFunc: func(context.Context) (*http.Response, error) {
			return nil, errors.New("provider says private things")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather-grid/latest"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "private things")
}

func TestWeatherGridObjectRequiresBothTimestamps(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{})

	for name, query := range map[string]string{
		"missing both":        "",
		"missing validTime":   "referenceTime=2026-09-05T12:00:00Z",
		"missing reference":   "validTime=2026-09-05T15:00:00Z",
		"malformed reference": "referenceTime=not-a-time&validTime=2026-09-05T15:00:00Z",
	} {
		t.Run(name, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather-grid/object?"+query))
			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestWeatherGridObjectForwardsTheParsedTimestampsAndTheRangeHeader(t *testing.T) {
	var gotReference, gotValid time.Time
	var gotMethod, gotRange string
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(
			_ context.Context, referenceTime, validTime time.Time, method, rangeHeader string,
		) (*http.Response, error) {
			gotReference, gotValid, gotMethod, gotRange = referenceTime, validTime, method, rangeHeader
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Range", "bytes 0-9/100")
			recorder.WriteHeader(http.StatusPartialContent)
			_, err := recorder.WriteString("0123456789")
			require.NoError(t, err)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	request := authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	request.Header.Set("Range", "bytes=0-9")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusPartialContent, response.Code)
	assert.True(t, gotReference.Equal(time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)))
	assert.True(t, gotValid.Equal(time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)))
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "bytes=0-9", gotRange)
	assert.Equal(t, "bytes 0-9/100", response.Header().Get("Content-Range"))
	assert.Equal(t, "0123456789", response.Body.String())
}

func TestWeatherGridObjectAnswersHeadWithNoBody(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(
			_ context.Context, _, _ time.Time, method, _ string,
		) (*http.Response, error) {
			assert.Equal(t, http.MethodHead, method)
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Length", "12345")
			recorder.WriteHeader(http.StatusOK)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	request := authenticatedRequest(http.MethodHead,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "12345", response.Header().Get("Content-Length"))
	assert.Empty(t, response.Body.String())
}

func TestWeatherGridObjectRefusesAResponseLargerThanTheLimit(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, string) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			// A promise this relay cannot keep without buffering the whole
			// body, which it deliberately never does — refusing up front is
			// what stops a client from being handed that promise and a
			// truncated body underneath it.
			recorder.Header().Set("Content-Length", "9999999999")
			recorder.WriteHeader(http.StatusOK)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.Empty(t, response.Header().Get("Content-Length"))
}

func TestWeatherGridObjectAllowsAHeadWithALargeContentLength(t *testing.T) {
	// HEAD carries no body to truncate, so the same Content-Length that a GET
	// must refuse is exactly what a HEAD exists to report.
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, string) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Length", "9999999999")
			recorder.WriteHeader(http.StatusOK)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodHead,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "9999999999", response.Header().Get("Content-Length"))
}

func TestWeatherGridObjectMapsANonSuccessUpstreamStatusToACleanError(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, string) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Type", "application/xml")
			recorder.WriteHeader(http.StatusForbidden)
			_, err := recorder.WriteString("<Error><Message>bucket policy details</Message></Error>")
			require.NoError(t, err)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "bucket policy details")
	assert.NotEqual(t, "application/xml", response.Header().Get("Content-Type"))
}

func TestWeatherGridObjectReturnsBadGatewayOnProviderFailure(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, string) (*http.Response, error) {
			return nil, errors.New("provider says private things")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "private things")
}
