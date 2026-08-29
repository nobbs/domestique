package komoot

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestClientProviderIsKomoot(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://example.invalid", Email: []byte("rider@example.invalid"), Password: []byte("secret")})
	require.NoError(t, err, "New()")

	assert.Equal(t, route.ProviderKomoot, client.Provider(), "Provider()")
}

func TestClientNeverSendsAnAcceptHeaderTheRealAPIRejects(t *testing.T) {
	// Verified against a live account: every v007 resource is served as HAL and
	// answers a strict "Accept: application/json" with 406. This server reproduces
	// that rejection so a regression fails loudly rather than only in production.
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasPrefix(request.URL.Path, "/v007/") && !strings.Contains(request.Header.Get("Accept"), "hal+json") {
			writer.WriteHeader(http.StatusNotAcceptable)
			writeJSON(t, writer, `{"status":406,"error":"HttpMediaTypeNotAcceptable","message":null}`)
			return
		}
		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
		case "/v007/users/42/tours/":
			writeJSON(t, writer, `{"_embedded":{"tours":[]},"page":{"size":50,"totalElements":0,"totalPages":0,"number":0}}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	stages, err := newTestClient(t, server).Inventory(t.Context())
	require.NoError(t, err)
	assert.Empty(t, stages)
}

func TestClientInventoryReadsPlannedToursAcrossPages(t *testing.T) {
	var methods []string
	var mu sync.Mutex
	var loginCount atomic.Int32

	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		methods = append(methods, request.Method)
		mu.Unlock()

		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			assertBasicAuth(t, request, "rider@example.test", "test-password")
			loginCount.Add(1)
			writeJSON(t, writer, `{"username":"42","password":"session-token","user":{"displayname":"Rider"}}`)
		case "/v007/users/42/tours/":
			assertBasicAuth(t, request, "42", "session-token")
			assert.Equal(t, "tour_planned", request.URL.Query().Get("type"), "type filter")
			switch request.URL.Query().Get("page") {
			case "0":
				writeJSON(t, writer, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":2,"totalPages":2,"number":0}}`)
			case "1":
				writeJSON(t, writer, `{"_embedded":{"tours":[{"id":101}]},"page":{"size":1,"totalElements":2,"totalPages":2,"number":1}}`)
			default:
				assert.Failf(t, "unexpected page", "%s", request.URL.Query().Get("page"))
			}
		case "/v007/tours/100":
			assertBasicAuth(t, request, "42", "session-token")
			assert.Equal(t, "coordinates", request.URL.Query().Get("_embedded"))
			writeJSON(t, writer, `{
                "id": 100, "type": "tour_planned", "name": "Alpine loop",
                "changed_at": "2026-08-17T07:00:00.000Z",
                "_embedded": {"coordinates": {"items": [
                    {"lat": 47.5, "lng": 10.2, "alt": 800},
                    {"lat": 47.6, "lng": 10.3, "alt": 850}
                ]}}
            }`)
		case "/v007/tours/101":
			assertBasicAuth(t, request, "42", "session-token")
			writeJSON(t, writer, `{
                "id": 101, "type": "tour_planned", "name": "  Ridge run  ",
                "changed_at": "2026-08-17T08:00:00.000Z",
                "_embedded": {"coordinates": {"items": [
                    {"lat": 48.0, "lng": 11.0, "alt": 400},
                    {"lat": 48.1, "lng": 11.1, "alt": 420}
                ]}}
            }`)
		default:
			assert.Failf(t, "unexpected request", "%s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	stages, err := client.Inventory(t.Context())
	require.NoError(t, err)
	require.Len(t, stages, 2)

	assert.Equal(t, "domestique:komoot:100:stage:1", stages[0].Key().ExternalID())
	assert.Equal(t, "Alpine loop", stages[0].Title())
	assert.Len(t, stages[0].ContentHash(), 64, "the content hash is a hex SHA-256 digest")

	geometry := stages[0].Geometry()
	require.Len(t, geometry, 2)
	assert.InDelta(t, 47.5, geometry[0].Latitude, 1e-9)
	assert.InDelta(t, 10.2, geometry[0].Longitude, 1e-9)
	if assert.NotNil(t, geometry[0].Elevation) {
		assert.InDelta(t, 800, *geometry[0].Elevation, 1e-9)
	}

	assert.Equal(t, "domestique:komoot:101:stage:1", stages[1].Key().ExternalID())
	assert.Equal(t, "Ridge run", stages[1].Title(), "the trimmed name is used")

	_, err = client.Inventory(t.Context())
	require.NoError(t, err, "second Inventory()")
	assert.Equal(t, int32(2), loginCount.Load(), "each run must authenticate its own session rather than reusing a cached token")

	mu.Lock()
	defer mu.Unlock()
	for _, method := range methods {
		assert.Equal(t, http.MethodGet, method, "komoot's session token permits writes; this package must never issue one")
	}
}

func TestClientInventoryRejectsUnauthenticatedLogin(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{name: "unknown account", status: http.StatusNotFound},
		{name: "wrong password", status: http.StatusUnauthorized},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(test.status)
				writeJSON(t, writer, `{"message":"not found"}`)
			}))
			defer server.Close()

			_, err := newTestClient(t, server).Inventory(t.Context())
			require.ErrorIs(t, err, ErrAuthentication)
		})
	}
}

func TestClientInventoryRejectsShortListing(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":5,"totalPages":1,"number":0}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "did not match pagination")
}

func TestClientInventoryRejectsWrongTourType(t *testing.T) {
	server := authenticatedTourServer(t, `{
        "id": 100, "type": "tour_recorded", "name": "Morning ride",
        "changed_at": "2026-08-17T07:00:00.000Z",
        "_embedded": {"coordinates": {"items": [{"lat": 1, "lng": 2, "alt": 3}, {"lat": 1.1, "lng": 2.1, "alt": 3.1}]}}
    }`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "not a planned tour")
}

func TestClientInventoryRejectsMissingAltitude(t *testing.T) {
	server := authenticatedTourServer(t, `{
        "id": 100, "type": "tour_planned", "name": "Morning ride",
        "changed_at": "2026-08-17T07:00:00.000Z",
        "_embedded": {"coordinates": {"items": [{"lat": 1, "lng": 2}, {"lat": 1.1, "lng": 2.1, "alt": 3.1}]}}
    }`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "missing a coordinate field")
}

func TestClientInventoryRejectsMissingRevision(t *testing.T) {
	server := authenticatedTourServer(t, `{
        "id": 100, "type": "tour_planned", "name": "Morning ride", "changed_at": "",
        "_embedded": {"coordinates": {"items": [{"lat": 1, "lng": 2, "alt": 3}, {"lat": 1.1, "lng": 2.1, "alt": 3.1}]}}
    }`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "source revision")
}

func TestClientInventoryRejectsOversizedBody(t *testing.T) {
	client, err := New(&Options{
		BaseURL:  "https://komoot.example.test",
		Email:    []byte("rider@example.test"),
		Password: []byte("test-password"),
		Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
			require.Equal(t, "/v006/account/email/rider@example.test/", request.URL.Path)

			// Streamed rather than built: the client reads the limit into
			// memory itself, and a materialised fixture would hold a second
			// copy of the same sixteen megabytes for the whole test.
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(io.LimitReader(zeroReader{}, maximumBodyBytes+1)),
				Header:     make(http.Header),
				Request:    request,
			}, nil
		}),
	})
	require.NoError(t, err)

	_, err = client.Inventory(t.Context())
	require.ErrorContains(t, err, "exceeded size limit")
}

func TestClientInventoryRejectsCancelledContext(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := newTestClient(t, server).Inventory(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestClientInventoryRejectsContextCancelledBetweenListingAndDetail(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
		case "/v007/users/42/tours/":
			cancel()
			writeJSON(t, writer, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":1,"totalPages":1,"number":0}}`)
		default:
			assert.Failf(t, "unexpected request", "%s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(ctx)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestClientInventoryRejectsInvalidTourID(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[{"id":0}]},"page":{"size":1,"totalElements":1,"totalPages":1,"number":0}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "invalid tour id")
}

func TestClientInventoryRejectsDetailFetchFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
		case "/v007/users/42/tours/":
			writeJSON(t, writer, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":1,"totalPages":1,"number":0}}`)
		case "/v007/tours/100":
			writer.WriteHeader(http.StatusInternalServerError)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "retrieving tour detail")
}

func TestClientInventoryRejectsMismatchedTourIdentity(t *testing.T) {
	server := authenticatedTourServer(t, `{
        "id": 999, "type": "tour_planned", "name": "Wrong tour",
        "changed_at": "2026-08-17T07:00:00.000Z",
        "_embedded": {"coordinates": {"items": [{"lat": 1, "lng": 2, "alt": 3}, {"lat": 1.1, "lng": 2.1, "alt": 3.1}]}}
    }`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "identity did not match")
}

func TestClientInventoryRejectsUnexpectedLoginStatus(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "login returned HTTP")
}

func TestClientInventoryRejectsMalformedLoginResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, `not json`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "not valid JSON")
}

func TestClientInventoryRejectsEmptyLoginCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, `{"username":"","password":""}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorIs(t, err, ErrAuthentication)
}

func TestClientInventoryRejectsNonNumericUsername(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, `{"username":"42/../evil","password":"session-token"}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorIs(t, err, ErrAuthentication)
}

func TestClientInventoryRejectsRedirect(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "http://insecure.example.invalid/leak", http.StatusFound)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "redirect")
}

func TestClientInventoryRejectsInconsistentEmptyPagination(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":1,"totalPages":0,"number":0}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "invalid tour library pagination")
}

func TestClientInventoryRejectsMissingPageObject(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[]}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "no page metadata")
}

func TestClientInventoryRejectsZeroElementsWithNonzeroPages(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[]},"page":{"size":50,"totalElements":0,"totalPages":3,"number":0}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "invalid tour library pagination")
}

func TestClientInventoryRejectsMissingToursContainer(t *testing.T) {
	server := authenticatedListingServer(t, `{"page":{"size":50,"totalElements":0,"totalPages":0,"number":0}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "no tours container")
}

func TestClientInventoryRejectsMissingPageSize(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[]},"page":{"totalElements":0,"totalPages":0,"number":0}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "no page metadata")
}

func TestClientInventoryRejectsInconsistentTotalPages(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
		case "/v007/users/42/tours/":
			switch request.URL.Query().Get("page") {
			case "0":
				writeJSON(t, writer, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":2,"totalPages":2,"number":0}}`)
			case "1":
				writeJSON(t, writer, `{"_embedded":{"tours":[{"id":101}]},"page":{"size":1,"totalElements":2,"totalPages":1,"number":1}}`)
			default:
				writer.WriteHeader(http.StatusNotFound)
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "invalid tour library pagination")
}

func TestClientInventoryRejectsZeroUsername(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, `{"username":"0","password":"session-token"}`)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorIs(t, err, ErrAuthentication)
}

func TestClientInventoryEscapesEmailExactlyOnce(t *testing.T) {
	const email = "ridér@example.test"
	var gotPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if gotPath == "" {
			gotPath = request.URL.Path
		}
		writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
	}))
	defer server.Close()

	client, err := New(&Options{
		BaseURL:   server.URL,
		Email:     []byte(email),
		Password:  []byte("test-password"),
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	_, err = client.Inventory(t.Context())
	require.ErrorContains(t, err, "no page metadata", "the stub server has no listing shape; only the captured login path matters here")
	assert.Equal(t, "/v006/account/email/"+email+"/", gotPath,
		"the earlier version pre-escaped the email and then let url.URL escape it a second time")
}

func TestClientInventoryTransportFailureDoesNotLeakEmail(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachableURL := server.URL
	server.Close()

	client, err := New(&Options{
		BaseURL:  unreachableURL,
		Email:    []byte("secret-rider@example.test"),
		Password: []byte("test-password"),
		Timeout:  time.Second,
	})
	require.NoError(t, err)

	_, err = client.Inventory(t.Context())
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret-rider@example.test",
		"http.Client wraps a dial failure in a *url.Error that embeds the request URL")
}

func TestClientInventoryPreservesEmailContainingSlash(t *testing.T) {
	const email = "a/b@example.test"
	var gotEscapedPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// request.URL.Path is already percent-decoded by net/http, so it reads
		// identically whether the client escaped the '/' correctly or sent it
		// as a literal separator — this test would pass either way if it
		// asserted on Path. EscapedPath (and the raw RequestURI) is the one
		// place the two cases still differ: %2F versus a literal '/'.
		if gotEscapedPath == "" {
			gotEscapedPath = request.URL.EscapedPath()
		}
		writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
	}))
	defer server.Close()

	client, err := New(&Options{
		BaseURL:   server.URL,
		Email:     []byte(email),
		Password:  []byte("test-password"),
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	_, err = client.Inventory(t.Context())
	require.ErrorContains(t, err, "no page metadata")
	assert.Equal(t, "/v006/account/email/a%2Fb@example.test/", gotEscapedPath,
		"a literal '/' in the email must arrive on the wire as %2F, one path segment, not a second real segment")
}

func TestClientInventoryRejectsPageObjectMissingFields(t *testing.T) {
	server := authenticatedListingServer(t, `{"_embedded":{"tours":[]},"page":{}}`)
	defer server.Close()

	_, err := newTestClient(t, server).Inventory(t.Context())
	require.ErrorContains(t, err, "no page metadata")
}

func TestNewRejectsNilOptions(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err)
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	tests := []struct {
		name    string
		options Options
	}{
		{
			name:    "non HTTPS base URL",
			options: Options{BaseURL: "http://komoot.example.test", Email: []byte("email"), Password: []byte("password")},
		},
		{
			name:    "base URL with no hostname",
			options: Options{BaseURL: "https://:443", Email: []byte("email"), Password: []byte("password")},
		},
		{
			name:    "missing email",
			options: Options{BaseURL: "https://komoot.example.test", Password: []byte("password")},
		},
		{
			name:    "missing password",
			options: Options{BaseURL: "https://komoot.example.test", Email: []byte("email")},
		},
		{
			name:    "negative timeout",
			options: Options{BaseURL: "https://komoot.example.test", Email: []byte("email"), Password: []byte("password"), Timeout: -time.Second},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := New(&test.options)
			require.Error(t, err)
		})
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:   server.URL,
		Email:     []byte("rider@example.test"),
		Password:  []byte("test-password"),
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	return client
}

func authenticatedListingServer(t *testing.T, listing string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
		case "/v007/users/42/tours/":
			writeJSON(t, writer, listing)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

func authenticatedTourServer(t *testing.T, detail string) *httptest.Server {
	t.Helper()
	return httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v006/account/email/rider@example.test/":
			writeJSON(t, writer, `{"username":"42","password":"session-token"}`)
		case "/v007/users/42/tours/":
			writeJSON(t, writer, `{"_embedded":{"tours":[{"id":100}]},"page":{"size":1,"totalElements":1,"totalPages":1,"number":0}}`)
		case "/v007/tours/100":
			writeJSON(t, writer, detail)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
}

func assertBasicAuth(t *testing.T, request *http.Request, wantUser, wantPassword string) {
	t.Helper()
	user, password, ok := request.BasicAuth()
	if !assert.True(t, ok, "request did not carry HTTP Basic credentials") {
		return
	}
	assert.Equal(t, wantUser, user, "basic auth user")
	assert.Equal(t, wantPassword, password, "basic auth password")
}

func writeJSON(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	_, err := io.WriteString(writer, body)
	assert.NoError(t, err, "writing the test response")
}

// zeroReader streams an endless run of zero bytes, so an oversized-body test
// needs no large fixture in the repository.
type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	for index := range buffer {
		buffer[index] = '0'
	}

	return len(buffer), nil
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
