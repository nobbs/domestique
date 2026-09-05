package openmeteogrid

import (
	"context"
	"net/http"
	"net/http/httptest"
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

	response, err := newTestClient(t, server).Latest(t.Context())
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
	response, err := newTestClient(t, server).Object(t.Context(), reference, valid, http.MethodGet, "")
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
		t.Context(), reference, valid, http.MethodHead, "bytes=0-99",
	)
	require.NoError(t, err)
	defer func() { assert.NoError(t, response.Body.Close()) }()
	assert.Equal(t, http.MethodHead, gotMethod)
	assert.Equal(t, "bytes=0-99", gotRange)
	assert.Equal(t, http.StatusPartialContent, response.StatusCode)
	assert.Equal(t, "bytes 0-99/200", response.Header.Get("Content-Range"))
}

func TestRequestsAskTheUpstreamNotToCompressTheBody(t *testing.T) {
	var gotAcceptEncoding string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		gotAcceptEncoding = request.Header.Get("Accept-Encoding")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	response, err := newTestClient(t, server).Latest(t.Context())
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
	response, err := client.Object(t.Context(), time.Now(), time.Now(), http.MethodPost, "")
	require.Error(t, err)
	require.Nil(t, response)
}

func TestUpstreamStatusPassesThroughRatherThanBecomingAnError(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	response, err := newTestClient(t, server).Latest(t.Context())
	require.NoError(t, err, "a non-2xx upstream status is the caller's to relay, not this package's to fail on")
	defer func() { assert.NoError(t, response.Body.Close()) }()
	assert.Equal(t, http.StatusNotFound, response.StatusCode)
}

func TestARequestThatCannotReachTheUpstreamIsAnError(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://127.0.0.1:1", Timeout: 200 * time.Millisecond})
	require.NoError(t, err)

	//nolint:bodyclose // A transport failure never returns a response to close.
	response, err := client.Latest(t.Context())
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
	response, err := newTestClient(t, server).Latest(ctx)
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
