package openmeteogrid

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRejectsNilOptions(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err)
}

func TestNewUsesTheDefaultBaseURLWhenNoneIsGiven(t *testing.T) {
	client, err := New(&Options{})
	require.NoError(t, err)
	assert.Equal(t, defaultBaseURL, client.baseURL.String())
}

func TestNewRejectsANegativeTimeout(t *testing.T) {
	_, err := New(&Options{BaseURL: "https://example.test", Timeout: -time.Second})
	require.Error(t, err)
}

func TestNewRejectsAMalformedBaseURL(t *testing.T) {
	_, err := New(&Options{BaseURL: "http://example.test"})
	require.Error(t, err)
}

func TestLatestFetchesTheModelsOwnManifest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "/data_spatial/dwd_icon_d2/latest.json", request.URL.Path)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	response, err := newTestClient(t, server).Latest(t.Context(), nil)
	require.NoError(t, err)
	defer func() { assert.NoError(t, response.Body.Close()) }()
	assert.Equal(t, http.StatusOK, response.StatusCode)
}

func TestObjectBuildsTheKeyFromTheRunHourAndTheValidStamp(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(
			t, "/data_spatial/dwd_icon_d2/2026/09/05/1200Z/2026-09-05T1500.om", request.URL.Path,
		)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// A reference time carrying non-zero minutes must still land on the hour:
	// the bucket's own directories are always "HH00Z", never the exact minute.
	reference := time.Date(2026, 9, 5, 12, 30, 0, 0, time.UTC)
	valid := time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	response, err := newTestClient(t, server).Object(t.Context(), reference, valid, http.MethodGet, nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())
}

func TestObjectForwardsTheRangeHeaderAndTheMethod(t *testing.T) {
	var gotMethod, gotRange string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotMethod = request.Method
		gotRange = request.Header.Get("Range")
		writer.Header().Set("Content-Range", "bytes 0-99/200")
		writer.WriteHeader(http.StatusPartialContent)
	}))
	defer server.Close()

	reference := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	valid := time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	response, err := newTestClient(t, server).Object(
		t.Context(), reference, valid, http.MethodHead, http.Header{"Range": {"bytes=0-99"}},
	)
	require.NoError(t, err)
	defer func() { assert.NoError(t, response.Body.Close()) }()
	assert.Equal(t, http.MethodHead, gotMethod)
	assert.Equal(t, "bytes=0-99", gotRange)
	assert.Equal(t, http.StatusPartialContent, response.StatusCode)
	assert.Equal(t, "bytes 0-99/200", response.Header.Get("Content-Range"))
}

func TestObjectForwardsConditionalHeadersAndNothingElse(t *testing.T) {
	var gotHeader http.Header
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotHeader = request.Header.Clone()
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	reference := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	valid := time.Date(2026, 9, 5, 15, 0, 0, 0, time.UTC)
	response, err := newTestClient(t, server).Object(t.Context(), reference, valid, http.MethodGet, http.Header{
		"Range":             {"bytes=0-99"},
		"If-None-Match":     {`"abc123"`},
		"If-Modified-Since": {"Wed, 09 Sep 2026 00:00:00 GMT"},
		"If-Range":          {`"abc123"`},
		"Cookie":            {"session=secret"},
		"Authorization":     {"Bearer secret"},
	})
	require.NoError(t, err)
	defer func() { assert.NoError(t, response.Body.Close()) }()
	assert.Equal(t, "bytes=0-99", gotHeader.Get("Range"))
	assert.Equal(t, `"abc123"`, gotHeader.Get("If-None-Match"))
	assert.Equal(t, "Wed, 09 Sep 2026 00:00:00 GMT", gotHeader.Get("If-Modified-Since"))
	assert.Equal(t, `"abc123"`, gotHeader.Get("If-Range"))
	assert.Empty(t, gotHeader.Get("Cookie"))
	assert.Empty(t, gotHeader.Get("Authorization"))
}

func TestRequestsAskTheUpstreamNotToCompressTheBody(t *testing.T) {
	var gotAcceptEncoding string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAcceptEncoding = request.Header.Get("Accept-Encoding")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := newTestClient(t, server).Latest(t.Context(), nil)
	require.NoError(t, err)
	defer func() { assert.NoError(t, response.Body.Close()) }()
	// net/http transparently gzips and decompresses otherwise, desyncing the
	// relayed ETag/Content-Length from the body this package must pass
	// through byte-for-byte.
	assert.Equal(t, "identity", gotAcceptEncoding)
}

func TestObjectRefusesAnyMethodButGetOrHead(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://example.test"})
	require.NoError(t, err)

	//nolint:bodyclose // The method check refuses before any request is made; the response is always nil here.
	response, err := client.Object(t.Context(), time.Now(), time.Now(), http.MethodPost, nil)
	require.Error(t, err)
	require.Nil(t, response)
}

func TestUpstreamStatusPassesThroughRatherThanBecomingAnError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	response, err := newTestClient(t, server).Latest(t.Context(), nil)
	require.NoError(t, err, "a non-2xx upstream status is the caller's to relay, not this package's to fail on")
	defer func() { assert.NoError(t, response.Body.Close()) }()
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestARequestThatCannotReachTheUpstreamIsAnError(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://127.0.0.1:1", Timeout: 200 * time.Millisecond})
	require.NoError(t, err)

	//nolint:bodyclose // A transport failure never returns a response to close.
	response, err := client.Latest(t.Context(), nil)
	require.Error(t, err)
	require.Nil(t, response)
	// The client never sees this detail — httpapi maps every such error to a
	// generic 502 — but it belongs in server logs, not just "request failed".
	assert.Contains(t, err.Error(), "connection refused")
}

func TestACancelledContextIsAnError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	//nolint:bodyclose // A cancelled context never returns a response to close.
	response, err := newTestClient(t, server).Latest(ctx, nil)
	require.Error(t, err)
	require.Nil(t, response)
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:   server.URL,
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	return client
}

func TestLatestServesASecondCallWithinTheTTLFromCache(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.Header().Set("ETag", `"abc"`)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	client, err := New(&Options{
		BaseURL: server.URL, Timeout: time.Second, Transport: server.Client().Transport,
		Now: func() time.Time { return now }, LatestTTL: time.Minute,
	})
	require.NoError(t, err)

	response, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())

	response, err = client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())

	assert.EqualValues(t, 1, requests.Load(), "the second call must be served from cache")
}

func TestLatestRefetchesAfterTheTTLExpires(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	client, err := New(&Options{
		BaseURL: server.URL, Timeout: time.Second, Transport: server.Client().Transport,
		Now: func() time.Time { return now }, LatestTTL: time.Minute,
	})
	require.NoError(t, err)

	response, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())

	now = now.Add(time.Minute + time.Second)
	response, err = client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())

	assert.EqualValues(t, 2, requests.Load(), "a call after the TTL must reach the upstream again")
}

func TestLatestCollapsesConcurrentMissesIntoOneUpstreamRequest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	const callers = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			<-start
			response, callErr := client.Latest(t.Context(), nil)
			if assert.NoError(t, callErr) {
				assert.NoError(t, response.Body.Close())
			}
		}()
	}
	close(start)
	wg.Wait()

	assert.EqualValues(t, 1, requests.Load(), "ten concurrent misses must collapse into one request")
}

func TestLatestDoesNotCacheA5xx(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	response, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())
	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)

	response, err = client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, response.Body.Close())

	assert.EqualValues(t, 2, requests.Load(), "an errored answer must not be remembered")
}

func TestLatestAnswersAMatchingIfNoneMatchWithA304FromCache(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.Header().Set("ETag", `"abc123"`)
		writer.Header().Set("Last-Modified", "Wed, 09 Sep 2026 00:00:00 GMT")
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	first, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, first.Body.Close())

	matching, err := client.Latest(t.Context(), http.Header{"If-None-Match": {`"abc123"`}})
	require.NoError(t, err)
	body, readErr := io.ReadAll(matching.Body)
	require.NoError(t, readErr)
	assert.NoError(t, matching.Body.Close())
	assert.Equal(t, http.StatusNotModified, matching.StatusCode)
	assert.Empty(t, body)
	assert.Equal(t, `"abc123"`, matching.Header.Get("ETag"))

	assert.EqualValues(t, 1, requests.Load(), "a 304 answer must still come from the cached manifest")
}

func TestLatestAnswersIfModifiedSinceFromTheCachedLastModified(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Last-Modified", "Wed, 09 Sep 2026 00:00:00 GMT")
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	first, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, first.Body.Close())

	current, err := client.Latest(t.Context(), http.Header{"If-Modified-Since": {"Wed, 09 Sep 2026 00:00:00 GMT"}})
	require.NoError(t, err)
	assert.NoError(t, current.Body.Close())
	assert.Equal(t, http.StatusNotModified, current.StatusCode)
	assert.Equal(t, "Wed, 09 Sep 2026 00:00:00 GMT", current.Header.Get("Last-Modified"))

	older, err := client.Latest(t.Context(), http.Header{"If-Modified-Since": {"Tue, 08 Sep 2026 00:00:00 GMT"}})
	require.NoError(t, err)
	assert.NoError(t, older.Body.Close())
	assert.Equal(t, http.StatusOK, older.StatusCode)

	// A sent If-None-Match decides on its own, even when the date would match.
	mismatch, err := client.Latest(t.Context(), http.Header{
		"If-None-Match": {`"other"`}, "If-Modified-Since": {"Wed, 09 Sep 2026 00:00:00 GMT"},
	})
	require.NoError(t, err)
	assert.NoError(t, mismatch.Body.Close())
	assert.Equal(t, http.StatusOK, mismatch.StatusCode)
}

func TestLatestAnswersAMismatchingIfNoneMatchWithTheCachedBody(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("ETag", `"abc123"`)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	first, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	assert.NoError(t, first.Body.Close())

	mismatch, err := client.Latest(t.Context(), http.Header{"If-None-Match": {`"other"`}})
	require.NoError(t, err)
	body, readErr := io.ReadAll(mismatch.Body)
	require.NoError(t, readErr)
	assert.NoError(t, mismatch.Body.Close())
	assert.Equal(t, http.StatusOK, mismatch.StatusCode)
	assert.JSONEq(t, `{"reference_time":"2026-09-05T12:00:00Z"}`, string(body))
}

func TestLatestCachedResponseBodyIsReadableAndClosableAcrossTwoSeparateHits(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	first, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	firstBody, err := io.ReadAll(first.Body)
	require.NoError(t, err)
	assert.NoError(t, first.Body.Close())

	second, err := client.Latest(t.Context(), nil)
	require.NoError(t, err)
	secondBody, err := io.ReadAll(second.Body)
	require.NoError(t, err)
	assert.NoError(t, second.Body.Close())

	assert.Equal(t, string(firstBody), string(secondBody))
	assert.JSONEq(t, `{"reference_time":"2026-09-05T12:00:00Z"}`, string(secondBody))
}

func TestLatestFetchesUnconditionallyOnAMissAndKeepsNoUpstreamLength(t *testing.T) {
	var gotIfNoneMatch string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotIfNoneMatch = request.Header.Get("If-None-Match")
		writer.Header().Set("ETag", `"abc"`)
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write([]byte(`{"reference_time":"2026-09-05T12:00:00Z"}`))
		assert.NoError(t, err)
	}))
	defer server.Close()

	response, err := newTestClient(t, server).Latest(t.Context(), http.Header{"If-None-Match": {`"abc"`}})
	require.NoError(t, err)
	body, readErr := io.ReadAll(response.Body)
	require.NoError(t, readErr)
	assert.NoError(t, response.Body.Close())

	// A follower sharing this miss may carry no validator, so the miss itself
	// never asks upstream for a 304; the caller's own is answered from the copy.
	assert.Empty(t, gotIfNoneMatch)
	assert.Equal(t, http.StatusNotModified, response.StatusCode)
	assert.Empty(t, body)
}

func TestLatestRefusesAManifestLargerThanItBuffers(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		_, err := writer.Write(bytes.Repeat([]byte("x"), maxManifestBodyBytes+1))
		assert.NoError(t, err)
	}))
	defer server.Close()

	//nolint:bodyclose // An oversized manifest is refused; no response is returned to close.
	response, err := newTestClient(t, server).Latest(t.Context(), nil)
	require.Error(t, err)
	assert.Nil(t, response)
}

func TestNewRejectsANegativeLatestTTL(t *testing.T) {
	_, err := New(&Options{LatestTTL: -time.Second})
	require.ErrorContains(t, err, "latest ttl")
}
