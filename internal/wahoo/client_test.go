package wahoo

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestClientCompletesOAuthAndFindsAuthenticatedUser(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/oauth/token":
			assert.Equal(t, http.MethodPost, request.Method, "token method")
			if !assert.NoError(t, request.ParseForm()) {
				return
			}
			assert.Equal(t, "client-id", request.Form.Get("client_id"), "client id")
			assert.Equal(t, "test-client-secret", request.Form.Get("client_secret"), "client secret")
			assert.Equal(t, "authorization-code", request.Form.Get("code"), "code")
			writeJSON(t, writer, map[string]string{"access_token": "access-token", "refresh_token": "refresh-token"})
		case "/v1/user":
			assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"), "authorization")
			writeJSON(t, writer, map[string]int64{"id": 42})
		default:
			assert.Failf(t, "unexpected request", "%s %s", request.Method, request.URL)
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	authorizationURL, err := client.AuthorizationURL("state-value")
	require.NoError(t, err)
	parsed, err := url.Parse(authorizationURL)
	require.NoError(t, err, "parsing the authorization URL")
	assert.Equal(t, "/oauth/authorize", parsed.Path, "authorization path")
	assert.Equal(t, "routes_read routes_write user_read", parsed.Query().Get("scope"), "authorization scope")
	assert.Equal(t, "state-value", parsed.Query().Get("state"), "authorization state")

	accessToken, refreshToken, err := client.ExchangeAuthorizationCode(t.Context(), "authorization-code")
	require.NoError(t, err)
	assert.Equal(t, "refresh-token", refreshToken)

	userID, err := client.AuthenticatedUser(t.Context(), accessToken)
	require.NoError(t, err)
	assert.Equal(t, "42", userID)
}

func TestClientWritesAndFindsOwnedRoute(t *testing.T) {
	stage := testStage(t)
	externalID := stage.Key().ExternalID()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/routes":
			switch request.Method {
			case http.MethodPost:
				assertRouteForm(t, request, externalID, true)
				writeJSON(t, writer, map[string]any{"id": 51, "external_id": externalID})
			case http.MethodGet:
				writeJSON(t, writer, []map[string]any{{"id": 51, "external_id": externalID}})
			default:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			}
		case "/v1/routes/51":
			switch request.Method {
			case http.MethodPut:
				assertRouteForm(t, request, externalID, false)
				writeJSON(t, writer, map[string]any{"id": 51, "external_id": externalID})
			case http.MethodDelete:
				writer.WriteHeader(http.StatusNoContent)
			default:
				writer.WriteHeader(http.StatusMethodNotAllowed)
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := newTestClient(t, server)
	createdRouteID, err := client.CreateRoute(t.Context(), "access-token", &stage, []byte("fit-data"))
	require.NoError(t, err)
	assert.Equal(t, int64(51), createdRouteID, "created route id")

	_, err = client.UpdateRoute(t.Context(), createdRouteID, "access-token", &stage, []byte("new-fit-data"))
	require.NoError(t, err)

	owned, err := client.ListOwnedRoutes(t.Context(), "access-token")
	require.NoError(t, err)
	require.Contains(t, owned, externalID, "the route this test just created was not listed")
	assert.Equal(t, int64(51), owned[externalID], "listed route id")

	require.NoError(t, client.DeleteRoute(t.Context(), createdRouteID, "access-token"))
}

func TestClientWaitsForAdvertisedRateLimit(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writer.Header().Set("X-RateLimit-Remaining", "10, 5, 0")
			writer.Header().Set("X-RateLimit-Reset", "5")
		}
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	var waited time.Duration
	client.wait = func(_ context.Context, duration time.Duration) error {
		waited = duration
		now = now.Add(duration)
		return nil
	}

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "first AuthenticatedUser()")
	_, err = client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "second AuthenticatedUser()")
	assert.Equal(t, 5*time.Second, waited, "the advertised reset was not waited out")
}

func TestClientReportsRateLimitObservedFromResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-RateLimit-Remaining", "187, 90, 20")
		writer.Header().Set("X-RateLimit-Reset", "120")
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }

	_, _, before := client.RateLimit()
	assert.False(t, before, "a client that has not made a request should report no known quota")

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)

	remaining, resetAt, ok := client.RateLimit()
	assert.True(t, ok, "quota should be known after a response carried one")
	assert.Equal(t, 20, remaining, "the lowest advertised window")
	assert.Equal(t, now.Add(120*time.Second), resetAt)
}

// A response that is not itself limited can report no usable reset at all —
// Wahoo answers seconds_until_reset as 0 on every such response — so the last
// advertised reset stays in place internally rather than being discarded. Once
// that time has actually passed, though, reporting it to a caller would be a
// refill time that has already gone by, which RateLimit clears rather than
// hands out.
func TestClientDoesNotReportAResetThatHasAlreadyPassed(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writer.Header().Set("X-RateLimit-Remaining", "50")
			writer.Header().Set("X-RateLimit-Reset", "1")
		} else {
			writer.Header().Set("X-RateLimit-Remaining", "49")
			writer.Header().Set("X-RateLimit-Reset", "0")
		}
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "first request")
	_, resetAt, _ := client.RateLimit()
	assert.Equal(t, now.Add(time.Second), resetAt, "the advertised reset")

	now = now.Add(2 * time.Second)
	_, err = client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "second request, past the first reset, carrying no reset of its own")

	remaining, resetAt, ok := client.RateLimit()
	assert.True(t, ok)
	assert.Equal(t, 49, remaining)
	assert.True(t, resetAt.IsZero(), "a reset already in the past must not be reported as due")
}

func TestClientListsOnlyRoutesCarryingAnExternalID(t *testing.T) {
	// A route with no external ID was created by hand in the Wahoo account.
	// Leaving it out of the map is what keeps it unmatchable, and so
	// undeletable, by reconciliation.
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, []map[string]any{
			{"id": 51, "external_id": "domestique:veloplanner:1:stage:1"},
			{"id": 52, "external_id": ""},
			{"id": 53},
			{"id": 0, "external_id": "domestique:veloplanner:9:stage:1"},
			{"id": 54, "external_id": "domestique:komoot:7:stage:1"},
		})
	}))
	defer server.Close()

	owned, err := newTestClient(t, server).ListOwnedRoutes(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, map[string]int64{
		"domestique:veloplanner:1:stage:1": 51,
		"domestique:komoot:7:stage:1":      54,
	}, owned, "only routes with both an external ID and a usable route ID belong in the map")
}

func TestClientRefusesAListingWithADuplicateExternalID(t *testing.T) {
	// Two routes claiming one external ID leaves no way to say which one this
	// service owns — a real possibility after a partially written run — so
	// every later answer about it would be decided by map order. Refuse the
	// listing instead of silently keeping whichever arrived last.
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, []map[string]any{
			{"id": 51, "external_id": "domestique:veloplanner:1:stage:1"},
			{"id": 52, "external_id": "domestique:veloplanner:1:stage:1"},
		})
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListOwnedRoutes(t.Context(), "access-token")
	require.ErrorContains(t, err, "duplicate external id")
}

func TestClientDeleteOwnedRoutesRemovesOnlyWhatItIssued(t *testing.T) {
	// Duplicates are the state a clear exists to get out of, so unlike the
	// reconciliation listing this must see both and remove both — while a
	// route it did not issue stays untouched.
	var deleted []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			deleted = append(deleted, request.URL.Path)
			writer.WriteHeader(http.StatusNoContent)

			return
		}
		writeJSON(t, writer, []map[string]any{
			{"id": 51, "external_id": "domestique:veloplanner:1:stage:1"},
			{"id": 52, "external_id": "domestique:veloplanner:1:stage:1"},
			{"id": 53, "external_id": ""},
			{"id": 54, "external_id": "strava:import:9"},
			{"id": 55, "external_id": "domestique:komoot:7:stage:1"},
		})
	}))
	defer server.Close()

	count, err := newTestClient(t, server).DeleteOwnedRoutes(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, 3, count, "deleted count")
	assert.Equal(t, []string{"/v1/routes/51", "/v1/routes/52", "/v1/routes/55"}, deleted,
		"only routes this service issued may be deleted, and every duplicate of them")
}

func TestClientDeleteOwnedRoutesWaitsOutASpentQuota(t *testing.T) {
	// A scheduled run would give up rather than sleep through a whole refill.
	// A clear is finished only when the target is empty, so it waits — the
	// alternative is pressing the same destructive button until it is done.
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodDelete {
			writer.Header().Set("X-RateLimit-Remaining", "100, 50, 0")
			writer.Header().Set("X-RateLimit-Reset", "300")
			writer.WriteHeader(http.StatusNoContent)

			return
		}
		writeJSON(t, writer, []map[string]any{
			{"id": 51, "external_id": "domestique:veloplanner:1:stage:1"},
			{"id": 52, "external_id": "domestique:veloplanner:2:stage:1"},
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	var waits []time.Duration
	client.wait = func(_ context.Context, duration time.Duration) error {
		waits = append(waits, duration)
		now = now.Add(duration)

		return nil
	}

	count, err := client.DeleteOwnedRoutes(t.Context(), "access-token")
	require.NoError(t, err, "a clear must ride out the quota rather than stop at it")
	assert.Equal(t, 2, count, "deleted count")
	assert.Equal(t, []time.Duration{5 * time.Minute}, waits, "the advertised reset was not waited out")
}

func TestClientRefusesAListingWithoutAnAccessToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		assert.Fail(t, "an unauthenticated listing must not reach the API")
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListOwnedRoutes(t.Context(), "")
	require.ErrorContains(t, err, "access token")
}

func TestClientReportsAFailedListing(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListOwnedRoutes(t.Context(), "access-token")
	require.ErrorContains(t, err, "HTTP 500")
}

func TestClientHoldsOffWhenAQuotaIsSpentWithNoResetHeaderAtAll(t *testing.T) {
	// Same reasoning as the zero-reset case, for a response that omits the
	// header outright: an unreadable reset must not read as "nothing to wait
	// for" while the quota itself is plainly spent.
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-RateLimit-Remaining", "100, 50, 0")
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "first AuthenticatedUser()")
	_, err = client.AuthenticatedUser(t.Context(), "access-token")
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestClientHoldsOffWhenAQuotaIsSpentWithoutAnAdvertisedReset(t *testing.T) {
	// Wahoo answers seconds_until_reset as 0 on any response that was not
	// itself limited, so a spent bucket routinely arrives with no usable
	// reset. Reading that as "no need to wait" is what previously left the
	// throttle permanently unarmed.
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("X-RateLimit-Remaining", "100, 50, 0")
		writer.Header().Set("X-RateLimit-Reset", "0")
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	client.wait = func(_ context.Context, duration time.Duration) error {
		now = now.Add(duration)

		return nil
	}

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "first AuthenticatedUser()")

	// The floor exceeds what one run will hold itself open for, so the next
	// request ends the run instead of sleeping through the whole window. The
	// run resumes from stored state on its next scheduled pass.
	_, err = client.AuthenticatedUser(t.Context(), "access-token")
	require.ErrorIs(t, err, ErrRateLimited)
}

func TestClientTransportFailureKeepsItsCauseWithoutTheRequestURL(t *testing.T) {
	// The cause is what makes a failed run diagnosable — a dial failure reads
	// very differently from a TLS one. The *url.Error wrapping it is not: its
	// message carries the request URL, which belongs in neither a log line nor
	// a notification.
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachable := server.URL
	server.Close()

	client, err := New(&Options{
		APIBaseURL:   unreachable,
		OAuthBaseURL: unreachable,
		ClientID:     "client-id",
		ClientSecret: []byte("client-secret"),
		RedirectURL:  "https://domestique.example.test/oauth/wahoo/callback",
		Timeout:      time.Second,
	})
	require.NoError(t, err, "New()")

	_, err = client.ListOwnedRoutes(t.Context(), "access-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect", "the underlying transport cause was discarded")
	assert.NotContains(t, err.Error(), unreachable, "the error carries the request URL")
}

func TestClientClassifiesRejectedRefreshToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).RefreshAccessToken(t.Context(), "refresh-token")
	require.ErrorIs(t, err, ErrUnauthorized)
}

// The token endpoint spends the same quota as everything else, so it has to
// wait out an advertised reset rather than going straight to Wahoo.
func TestClientThrottlesTheTokenEndpoint(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/user" {
			writer.Header().Set("X-RateLimit-Remaining", "10, 5, 0")
			writer.Header().Set("X-RateLimit-Reset", "5")
			writeJSON(t, writer, map[string]int64{"id": 42})

			return
		}
		writeJSON(t, writer, map[string]string{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	var waited time.Duration
	client.wait = func(_ context.Context, duration time.Duration) error {
		waited = duration
		now = now.Add(duration)

		return nil
	}

	// Spends the quota, which arms the throttle for whatever asks next.
	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "priming the rate limit")

	_, _, err = client.RefreshAccessToken(t.Context(), "refresh-token")
	require.NoError(t, err, "refreshing")
	assert.Equal(t, 5*time.Second, waited, "the token request did not wait out the advertised reset")
}

// Wahoo answers a rate-limited token request with 429, which reaches sync as
// ErrRateLimited rather than as an opaque failure.
func TestClientClassifiesRateLimitedTokenRequest(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).RefreshAccessToken(t.Context(), "refresh-token")
	require.ErrorIs(t, err, ErrRateLimited)
}

// x/oauth2 carries the refresh token it was given forward when a reply omits
// one, so a rotation that did not happen would otherwise be stored as if it had.
func TestClientRejectsATokenReplyMissingHalfOfIt(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]string{"access_token": "access-token"})
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).ExchangeAuthorizationCode(t.Context(), "authorization-code")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incomplete")
}

func TestClientRefusesIncompleteOAuthArguments(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		assert.Fail(t, "an incomplete argument must be refused before a request is sent")
		writer.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server)

	_, err := client.AuthorizationURL("")
	require.Error(t, err, "authorization URL without state")

	_, _, err = client.ExchangeAuthorizationCode(t.Context(), "")
	require.Error(t, err, "exchange without a code")

	_, _, err = client.RefreshAccessToken(t.Context(), "")
	require.Error(t, err, "refresh without a token")
}

// A token endpoint that cannot be reached is neither a rejection nor a rate
// limit, so it must not be reported as one.
func TestClientDoesNotMisclassifyAnUnreachableTokenEndpoint(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client := newTestClient(t, server)
	server.Close()

	_, _, err := client.RefreshAccessToken(t.Context(), "refresh-token")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrUnauthorized)
	require.NotErrorIs(t, err, ErrRateLimited)
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		APIBaseURL:   server.URL,
		OAuthBaseURL: server.URL,
		ClientID:     "client-id",
		RedirectURL:  "https://pi.example.ts.net/oauth/wahoo/callback",
		ClientSecret: []byte("test-client-secret"),
		Timeout:      time.Second,
		Transport:    server.Client().Transport,
	})
	require.NoError(t, err)

	return client
}

func testStage(t *testing.T) route.Route {
	t.Helper()
	elevation := 100.0
	stage, err := route.NewRoute(
		route.ProviderVeloPlanner,
		100,
		1,
		"2026-08-17T07:00:00Z",
		"Morning route",
		"",
		[]route.Point{
			{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
			{Longitude: 8.401, Latitude: 49.001, Elevation: &elevation},
		},
		"hash",
	)
	require.NoError(t, err)

	return stage
}

// assertRouteForm runs on the server's goroutine, so it asserts rather than
// requires: FailNow off the test goroutine would leave the request hanging.
func assertRouteForm(t *testing.T, request *http.Request, externalID string, expectExternalID bool) {
	t.Helper()
	if err := request.ParseForm(); err != nil {
		assert.Failf(t, "parsing the submitted route form", "%v", err)

		return
	}
	assert.True(t,
		strings.HasPrefix(request.Form.Get("route[file]"), "data:application/vnd.fit;base64,"),
		"route file is not a FIT data URI")
	assert.Equal(t, "Morning route", request.Form.Get("route[name]"), "route name")
	distance := request.Form.Get("route[distance]")
	assert.NotEmpty(t, distance, "route distance")
	assert.NotEqual(t, "0", distance, "route distance must be a positive value")
	if expectExternalID {
		assert.Equal(t, externalID, request.Form.Get("route[external_id]"), "route external id")
	} else {
		// An update names the route by id; resending the external id would let
		// a typo in it point the stage at somebody else's route.
		assert.Empty(t, request.Form.Get("route[external_id]"), "update external id")
	}
}

// writeJSON answers with a Content-Type because x/oauth2 reads it to choose
// between decoding JSON and a form body, as RFC 6749 requires of a token reply.
func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(writer).Encode(value), "writing the JSON response")
}
