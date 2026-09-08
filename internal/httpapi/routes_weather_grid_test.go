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
		LatestFunc: func(context.Context, http.Header) (*http.Response, error) {
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
		LatestFunc: func(context.Context, http.Header) (*http.Response, error) {
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
	var gotMethod string
	var gotConditional http.Header
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(
			_ context.Context, referenceTime, validTime time.Time, method string, conditional http.Header,
		) (*http.Response, error) {
			gotReference, gotValid, gotMethod, gotConditional = referenceTime, validTime, method, conditional
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
	assert.Equal(t, "bytes=0-9", gotConditional.Get("Range"))
	assert.Equal(t, "bytes 0-9/100", response.Header().Get("Content-Range"))
	assert.Equal(t, "0123456789", response.Body.String())
}

func TestWeatherGridObjectAnswersHeadWithNoBody(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(
			_ context.Context, _, _ time.Time, method string, _ http.Header,
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
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
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
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
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

func TestWeatherGridObjectMapsAnUnsatisfiableRangeToABadRequest(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Range", "bytes */100")
			recorder.WriteHeader(http.StatusRequestedRangeNotSatisfiable)

			return recorder.Result(), nil
		},
	}
	handler := newHandlerWithWeatherGrid(t, weatherGrid)

	request := authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	request.Header.Set("Range", "bytes=999999999-")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	// The caller's own request is what's wrong here, not the provider —
	// a 502 would tell a reader to retry a range that will never satisfy.
	require.Equal(t, http.StatusBadRequest, response.Code)
}

func TestWeatherGridObjectMapsANonSuccessUpstreamStatusToACleanError(t *testing.T) {
	weatherGrid := &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
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
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
			return nil, errors.New("provider says private things")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "private things")
}

func TestWeatherGridObjectCarriesTheImmutableCacheHeader(t *testing.T) {
	for name, method := range map[string]string{"GET": http.MethodGet, "HEAD": http.MethodHead} {
		t.Run(name, func(t *testing.T) {
			handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
				ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
					recorder := httptest.NewRecorder()
					recorder.WriteHeader(http.StatusOK)

					return recorder.Result(), nil
				},
			})

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(method,
				"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

			require.Equal(t, http.StatusOK, response.Code)
			assert.Equal(t, cacheWeatherGridObject, response.Header().Get("Cache-Control"))
		})
	}
}

func TestWeatherGridObjectPartialContentCarriesTheImmutableCacheHeader(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Range", "bytes 0-9/100")
			recorder.WriteHeader(http.StatusPartialContent)
			_, err := recorder.WriteString("0123456789")
			require.NoError(t, err)

			return recorder.Result(), nil
		},
	})

	request := authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	request.Header.Set("Range", "bytes=0-9")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusPartialContent, response.Code)
	assert.Equal(t, cacheWeatherGridObject, response.Header().Get("Cache-Control"))
}

func TestWeatherGridObjectRelaysAnUpstreamNotModified(t *testing.T) {
	var gotIfNoneMatch string
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		ObjectFunc: func(
			_ context.Context, _, _ time.Time, _ string, conditional http.Header,
		) (*http.Response, error) {
			gotIfNoneMatch = conditional.Get("If-None-Match")
			recorder := httptest.NewRecorder()
			recorder.Header().Set("ETag", `"abc123"`)
			recorder.Header().Set("Content-Length", "1024")
			recorder.Header().Set("Content-Range", "bytes 0-1023/4096")
			recorder.WriteHeader(http.StatusNotModified)
			result := recorder.Result()
			// A 304 may still name the full object's length; nothing follows it.
			result.ContentLength = maximumWeatherGridBytes + 1

			return result, nil
		},
	})

	request := authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	request.Header.Set("If-None-Match", `"abc123"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotModified, response.Code)
	assert.Equal(t, `"abc123"`, gotIfNoneMatch)
	assert.Empty(t, response.Body.String())
	assert.Equal(t, `"abc123"`, response.Header().Get("ETag"))
	assert.Empty(t, response.Header().Get("Content-Length"))
	assert.Empty(t, response.Header().Get("Content-Range"))
	assert.Equal(t, cacheWeatherGridObject, response.Header().Get("Cache-Control"))
}

func TestWeatherGridObjectForwardsRangeAndIfRangeAndStillAnswersPartialContent(t *testing.T) {
	var gotConditional http.Header
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		ObjectFunc: func(
			_ context.Context, _, _ time.Time, _ string, conditional http.Header,
		) (*http.Response, error) {
			gotConditional = conditional
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Range", "bytes 0-9/100")
			recorder.WriteHeader(http.StatusPartialContent)
			_, err := recorder.WriteString("0123456789")
			require.NoError(t, err)

			return recorder.Result(), nil
		},
	})

	request := authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	request.Header.Set("Range", "bytes=0-9")
	request.Header.Set("If-Range", `"abc123"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusPartialContent, response.Code)
	assert.Equal(t, "bytes=0-9", gotConditional.Get("Range"))
	assert.Equal(t, `"abc123"`, gotConditional.Get("If-Range"))
}

func TestWeatherGridObjectUnsatisfiableRangeStillCarriesNoStore(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.Header().Set("Content-Range", "bytes */100")
			recorder.WriteHeader(http.StatusRequestedRangeNotSatisfiable)

			return recorder.Result(), nil
		},
	})

	request := authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z")
	request.Header.Set("Range", "bytes=999999999-")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, cacheAPI, response.Header().Get("Cache-Control"))
}

func TestWeatherGridObjectBadGatewayStillCarriesNoStore(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		ObjectFunc: func(context.Context, time.Time, time.Time, string, http.Header) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusForbidden)

			return recorder.Result(), nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather-grid/object?referenceTime=2026-09-05T12:00:00Z&validTime=2026-09-05T15:00:00Z"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.Equal(t, cacheAPI, response.Header().Get("Cache-Control"))
}

func TestWeatherGridLatestCarriesTheShortLivedCacheHeader(t *testing.T) {
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		LatestFunc: func(context.Context, http.Header) (*http.Response, error) {
			recorder := httptest.NewRecorder()
			recorder.WriteHeader(http.StatusOK)

			return recorder.Result(), nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather-grid/latest"))

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, cacheWeatherGridLatest, response.Header().Get("Cache-Control"))
}

func TestWeatherGridLatestRelaysAnUpstreamNotModified(t *testing.T) {
	var gotConditional http.Header
	handler := newHandlerWithWeatherGrid(t, &fakeWeatherGrid{
		LatestFunc: func(_ context.Context, conditional http.Header) (*http.Response, error) {
			gotConditional = conditional
			recorder := httptest.NewRecorder()
			recorder.Header().Set("ETag", `"latest123"`)
			recorder.WriteHeader(http.StatusNotModified)

			return recorder.Result(), nil
		},
	})

	request := authenticatedRequest(http.MethodGet, "/v1/weather-grid/latest")
	request.Header.Set("If-None-Match", `"latest123"`)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusNotModified, response.Code)
	assert.Equal(t, `"latest123"`, gotConditional.Get("If-None-Match"))
	assert.Empty(t, response.Body.String())
	assert.Equal(t, cacheWeatherGridLatest, response.Header().Get("Cache-Control"))
}
