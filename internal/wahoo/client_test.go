package wahoo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"

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
	assert.Equal(t, "routes_read routes_write user_read workouts_read offline_data", parsed.Query().Get("scope"), "authorization scope")
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

// A response that is not itself limited reports no usable reset — Wahoo answers
// seconds_until_reset as 0 — so the last advertised reset stays in place
// internally. Once that time has passed, reporting it would be a refill time
// already gone by, which RateLimit clears.
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

// Only the token endpoint judges the refresh token. A 401 on a data read may be
// throttling, a scope gap or an upstream fault, none of which spend the grant.
func TestClientDoesNotClassifyARejectedReadAsUnauthorized(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListOwnedRoutes(t.Context(), "access-token")
	require.ErrorContains(t, err, "HTTP 401")
	require.NotErrorIs(t, err, ErrUnauthorized)
	require.NotErrorIs(t, err, ErrRateLimited)
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
	// The status is what tells a withdrawn grant from a throttled request once
	// the credential that would allow a retest has already been discarded.
	require.ErrorContains(t, err, "HTTP 400")
	assert.NotContains(t, err.Error(), "refresh-token")
}

// Wahoo exempts the token endpoint from its rate limits, so a refresh goes
// straight to Wahoo while the data quota is spent, and whatever quota headers
// its reply carries say nothing about the data quota this client observes.
func TestClientNeverThrottlesTheTokenEndpoint(t *testing.T) {
	var tokenRequests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/user" {
			writer.Header().Set("X-RateLimit-Remaining", "10, 5, 0")
			writer.Header().Set("X-RateLimit-Reset", "300")
			writeJSON(t, writer, map[string]int64{"id": 42})

			return
		}
		tokenRequests++
		// Headers that would plainly move the observed quota were they read:
		// a different remaining, and a reset the observer never ignores.
		writer.Header().Set("X-RateLimit-Remaining", "7, 7, 7")
		writer.Header().Set("X-RateLimit-Reset", "900")
		writeJSON(t, writer, map[string]string{
			"access_token":  "access-token",
			"refresh_token": "refresh-token",
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Date(2026, time.August, 17, 7, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return now }
	client.wait = func(_ context.Context, _ time.Duration) error {
		return errors.New("the token request waited for the data quota")
	}

	// Spends the quota, which arms the throttle for whatever data request asks next.
	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "priming the rate limit")
	require.True(t, client.notBefore.After(now), "the data quota was not spent")

	_, _, err = client.RefreshAccessToken(t.Context(), "refresh-token")
	require.NoError(t, err, "refreshing while the data quota is spent")
	assert.Equal(t, 1, tokenRequests, "the refresh did not reach the token endpoint")

	remaining, resetAt, ok := client.RateLimit()
	require.True(t, ok)
	assert.Zero(t, remaining, "the token response changed the observed quota")
	assert.Equal(t, now.Add(300*time.Second), resetAt, "the token response changed the observed reset")
	assert.Equal(t, now.Add(300*time.Second), client.notBefore, "the token response moved the throttle")
	_, err = client.AuthenticatedUser(t.Context(), "access-token")
	require.ErrorIs(t, err, ErrRateLimited, "a data request went ahead on a quota the token response had not refilled")
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

// Wahoo caps unrevoked access tokens per application and user and has no
// per-token revoke, so a token minted per run would eventually lock the account
// out of authorizing. One request, then reuse.
func TestClientReusesAHeldAccessToken(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		writeJSON(t, writer, map[string]any{
			"access_token": "access-1", "refresh_token": "refresh-token", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	for range 3 {
		access, rotated, err := client.RefreshAccessToken(t.Context(), "refresh-token")
		require.NoError(t, err)
		assert.Equal(t, "access-1", access, "access token")
		assert.Equal(t, "refresh-token", rotated, "refresh token")
	}
	assert.Equal(t, 1, requests, "Wahoo was asked for a token more than once")
}

// A held token is retired before Wahoo stops honouring it, so a caller is never
// handed one that expires part-way through its run.
func TestClientRetiresAHeldTokenBeforeItExpires(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		writeJSON(t, writer, map[string]any{
			"access_token": "access", "refresh_token": "refresh-token", "expires_in": 300, "token_type": "bearer"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Now()
	client.now = func() time.Time { return now }

	_, _, err := client.RefreshAccessToken(t.Context(), "refresh-token")
	require.NoError(t, err)
	// A 300s token with a 120s margin is spent to this client from 180s on; 200s
	// is past that without depending on when the reply was actually stamped.
	now = now.Add(200 * time.Second)

	_, _, err = client.RefreshAccessToken(t.Context(), "refresh-token")
	require.NoError(t, err)
	assert.Equal(t, 2, requests, "a token inside its expiry margin was reused")
}

// The rotated token is the only handle that still reaches the account, so the
// spent one must not keep an entry alive.
func TestClientHoldsARotatedTokenUnderItsNewKey(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		writeJSON(t, writer, map[string]any{
			"access_token": "access", "refresh_token": "rotated", "expires_in": 3600, "token_type": "bearer"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, rotated, err := client.RefreshAccessToken(t.Context(), "refresh-token")
	require.NoError(t, err)
	require.Equal(t, "rotated", rotated)

	_, _, err = client.RefreshAccessToken(t.Context(), rotated)
	require.NoError(t, err)
	assert.Equal(t, 1, requests, "the rotated token was not held")
	assert.Len(t, client.held, 1, "the spent refresh token still holds an entry")
}

// Without an expiry this client cannot know when a token stops working, so it
// holds nothing rather than holding it forever.
func TestClientHoldsNoTokenWithoutAnExpiry(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		writeJSON(t, writer, map[string]any{
			"access_token": "access", "refresh_token": "refresh-token", "token_type": "bearer"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	for range 2 {
		_, _, err := client.RefreshAccessToken(t.Context(), "refresh-token")
		require.NoError(t, err)
	}
	assert.Equal(t, 2, requests, "a token with no stated expiry was held anyway")
	assert.Empty(t, client.held, "a token with no stated expiry was held")
}

// x/oauth2 can raise a retrieval error carrying no response at all. Reporting
// that as HTTP 0 would read as a status Wahoo sent.
func TestClientReportsATokenRefusalCarryingNoResponse(t *testing.T) {
	err := classifyRetrieveError(&oauth2.RetrieveError{Body: []byte(`{"error":"invalid_grant"}`)})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "HTTP 0", "an absent response was reported as a status")
	assert.Contains(t, err.Error(), "without an HTTP response")
	var named interface{ Category() string }
	require.ErrorAs(t, err, &named)
	assert.Equal(t, "invalid_grant", named.Category(), "the reply still names the refusal")
}

// A status this package does not classify still reaches a log through sync, and
// x/oauth2's own error quotes the reply. Only the status crosses this boundary.
func TestClientDoesNotQuoteAnUnclassifiedTokenRefusal(t *testing.T) {
	const reply = "upstream said something quotable"
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		writeJSON(t, writer, map[string]string{"error": reply})
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).RefreshAccessToken(t.Context(), "refresh-token")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrUnauthorized, "a 500 is not a withdrawn grant")
	require.NotErrorIs(t, err, ErrRateLimited)
	assert.Contains(t, err.Error(), "HTTP 500", "the status a reader needs")
	assert.NotContains(t, err.Error(), reply, "the token endpoint's reply reached the error")
}

// A refresh that fails must not leave the spent token held. Nothing would reuse
// it, and nothing else would ever remove it — a withdrawn grant would keep a
// dead entry for as long as the process runs.
func TestClientDropsAHeldTokenItCanNoLongerUse(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		if requests == 1 {
			writeJSON(t, writer, map[string]any{
				"access_token": "access", "refresh_token": "refresh-token", "expires_in": 300, "token_type": "bearer"})

			return
		}
		writer.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	now := time.Now()
	client.now = func() time.Time { return now }

	_, _, err := client.RefreshAccessToken(t.Context(), "refresh-token")
	require.NoError(t, err)
	require.Len(t, client.held, 1, "the first token was not held")

	now = now.Add(200 * time.Second)
	_, _, err = client.RefreshAccessToken(t.Context(), "refresh-token")
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.Empty(t, client.held, "a token that can no longer be used is still held")
}

// Wahoo does not keep to the OAuth error codes: the refusal that matters most
// here arrives as an English sentence, and anything unrecognised is not quoted
// onward into a log.
func TestTokenErrorCategory(t *testing.T) {
	for name, test := range map[string]struct{ body, category string }{
		"exhausted quota": {
			body: `{"error":"Too many unrevoked access tokens exist for this app and user. ` +
				`You can only create a new token if you revoke an old one first."}`,
			category: "token_quota_exhausted",
		},
		"rejected grant":  {body: `{"error":"invalid_grant"}`, category: "invalid_grant"},
		"rejected client": {body: `{"error":"invalid_client"}`, category: "invalid_client"},
		"unknown wording": {body: `{"error":"something new"}`, category: "unrecognised"},
		"not json":        {body: `<html>gateway</html>`, category: "unrecognised"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.category, tokenErrorCategory([]byte(test.body)))
		})
	}
}

// The category rides along with the sentinel rather than replacing it: sync
// still reads ErrUnauthorized to know a target needs reauthorizing.
func TestClientNamesAnExhaustedTokenQuota(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusBadRequest)
		writeJSON(t, writer, map[string]string{"error": "Too many unrevoked access tokens exist for this app and user."})
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).RefreshAccessToken(t.Context(), "refresh-token")
	require.ErrorIs(t, err, ErrUnauthorized)
	var named interface{ Category() string }
	require.ErrorAs(t, err, &named)
	assert.Equal(t, "token_quota_exhausted", named.Category())
	assert.NotContains(t, err.Error(), "unrevoked", "the upstream wording reached the error")
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
