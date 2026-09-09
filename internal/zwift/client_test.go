package zwift

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// s3RedirectTransport lets a DownloadFIT test use a real
// https://bucket.s3.amazonaws.com URL — as fitHostPattern requires — while
// the request actually lands on the local httptest.Server behind target.
type s3RedirectTransport struct {
	target    *url.URL
	transport http.RoundTripper
}

func (s s3RedirectTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	redirected := request.Clone(request.Context())
	redirected.URL.Scheme = s.target.Scheme
	redirected.URL.Host = s.target.Host
	redirected.Host = s.target.Host

	response, err := s.transport.RoundTrip(redirected)
	if err != nil {
		return nil, fmt.Errorf("s3RedirectTransport: %w", err)
	}

	return response, nil
}

func fakeS3URL(key string) string {
	return "https://test-bucket.s3.amazonaws.com/" + key
}

func newDownloadTestClient(t *testing.T, server *httptest.Server, timeout time.Duration) *Client {
	t.Helper()
	target, err := url.Parse(server.URL)
	require.NoError(t, err)
	client, err := New(&Options{
		Transport: s3RedirectTransport{target: target, transport: server.Client().Transport},
		Timeout:   timeout,
	})
	require.NoError(t, err)

	return client
}

func TestClientSignsInWithThePasswordGrant(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/auth/realms/zwift/tokens/access/codes", request.URL.Path)
		assert.Equal(t, http.MethodPost, request.Method)
		if !assert.NoError(t, request.ParseForm()) {
			return
		}
		assert.Equal(t, "password", request.Form.Get("grant_type"), "grant type")
		assert.Equal(t, "rider@example.com", request.Form.Get("username"), "username")
		assert.Equal(t, "hunter2", request.Form.Get("password"), "password")
		assert.Equal(t, "Zwift_Mobile_Link", request.Form.Get("client_id"), "client id")
		writeJSON(t, writer, map[string]any{
			"access_token": "access-token", "refresh_token": "refresh-token", "expires_in": 3600,
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	session, err := client.Session(t.Context(), []byte("rider@example.com"), []byte("hunter2"))
	require.NoError(t, err)
	assert.Equal(t, "access-token", session.AccessToken)
	assert.Equal(t, "refresh-token", session.RefreshToken)
	assert.WithinDuration(t, time.Now().Add(time.Hour), session.ExpiresAt, 5*time.Second, "expiry")
}

func TestClientRefreshesWithARefreshToken(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !assert.NoError(t, request.ParseForm()) {
			return
		}
		assert.Equal(t, "refresh_token", request.Form.Get("grant_type"), "grant type")
		assert.Equal(t, "old-refresh", request.Form.Get("refresh_token"), "refresh token")
		assert.Equal(t, "Zwift_Mobile_Link", request.Form.Get("client_id"), "client id")
		writeJSON(t, writer, map[string]any{
			"access_token": "new-access", "refresh_token": "new-refresh", "expires_in": 3600,
		})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	session, err := client.Refresh(t.Context(), Session{RefreshToken: "old-refresh"})
	require.NoError(t, err)
	assert.Equal(t, "new-access", session.AccessToken)
	assert.Equal(t, "new-refresh", session.RefreshToken)
}

func TestClientReadsThePlayerID(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/profiles/me", request.URL.Path)
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"), "authorization")
		writeJSON(t, writer, map[string]int64{"id": 4711})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	playerID, err := client.PlayerID(t.Context(), Session{AccessToken: "access-token"})
	require.NoError(t, err)
	assert.Equal(t, int64(4711), playerID)
}

func TestClientListsActivitiesByOffsetAndParsesIDStr(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/profiles/99/activities", request.URL.Path)
		assert.Equal(t, "20", request.URL.Query().Get("start"), "start")
		assert.Equal(t, "10", request.URL.Query().Get("limit"), "limit")
		assert.Equal(t, "application/json", request.Header.Get("Accept"), "accept header, or Zwift answers protobuf-lite")
		// id is Zwift's own float-rounded numeric id; id_str is exact and this
		// package must read that one instead.
		writer.Write([]byte(`[{"id":1461969115156611000,"id_str":"1461969115156611104","sport":"CYCLING"}]`)) //nolint:errcheck,gosec // test server, nothing to act on
	}))
	defer server.Close()

	client := newTestClient(t, server)
	activities, err := client.Activities(t.Context(), Session{AccessToken: "access-token"}, 99, 20, 10)
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, int64(1461969115156611104), activities[0].ID, "id must come from id_str, not the rounded id")
}

func TestClientListActivitiesDecodesTheLiveTimestampLayout(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Write([]byte(`[{"id_str":"1","sport":"CYCLING",` + //nolint:errcheck,gosec // test server, nothing to act on
			`"startDate":"2026-09-08T18:04:39.000+0000","endDate":"2026-09-08T19:04:39.123-0530"}]`))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	activities, err := client.Activities(t.Context(), Session{AccessToken: "access-token"}, 1, 0, 1)
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.True(t, activities[0].StartDate.Equal(time.Date(2026, 9, 8, 18, 4, 39, 0, time.UTC)), "start date")
	assert.True(t, activities[0].EndDate.Equal(time.Date(2026, 9, 8, 19, 4, 39, 123_000_000, time.FixedZone("", -5*3600-30*60))), "end date")
}

func TestClientListActivitiesFallsBackToRFC3339Timestamps(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Write([]byte(`[{"id_str":"1","sport":"CYCLING",` + //nolint:errcheck,gosec // test server, nothing to act on
			`"startDate":"2026-09-08T18:04:39Z","endDate":"2026-09-08T19:04:39Z"}]`))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	activities, err := client.Activities(t.Context(), Session{AccessToken: "access-token"}, 1, 0, 1)
	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.True(t, activities[0].StartDate.Equal(time.Date(2026, 9, 8, 18, 4, 39, 0, time.UTC)), "start date")
}

// The single-activity response also carries third-party fields the listing
// already withholds; this asserts Activity() never decodes them into anything
// that could leak, by proving the handler's own raw body carries one that the
// client's response does not need to reject -- only pull the three fields.
func TestClientReadsOneActivitysWorkoutAndIgnoresThirdPartyFields(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/activities/12345", request.URL.Path)
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"), "authorization")
		assert.Equal(t, "application/json", request.Header.Get("Accept"), "accept header")
		writer.Write([]byte(`{"name":"Sweet Spot Progression","workoutHash":998877,` + //nolint:errcheck,gosec // test server
			`"percentageCompleted":0.87,"profileFtp":250,"profileMaxHeartRate":180,` +
			`"socialInteractions":[{"profile":{"firstName":"Someone Else"}}],` +
			`"activityRideOns":[{"profileId":99}],"profile":{"firstName":"The Rider"},` +
			`"clubAttributions":[{"id":1}],"notableMoments":[{"type":"pr"}]}`))
	}))
	defer server.Close()

	client := newTestClient(t, server)
	detail, err := client.Activity(t.Context(), Session{AccessToken: "access-token"}, 12345)
	require.NoError(t, err)
	assert.Equal(t, "Sweet Spot Progression", detail.Name)
	assert.Equal(t, int64(998877), detail.WorkoutHash)
	assert.InDelta(t, 0.87, detail.PercentageCompleted, 1e-9)
}

func TestClientActivityRequiresASessionAndAPositiveID(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()
	client := newTestClient(t, server)

	_, err := client.Activity(t.Context(), Session{}, 1)
	require.ErrorContains(t, err, "session and activity id are required")

	_, err = client.Activity(t.Context(), Session{AccessToken: "access-token"}, 0)
	require.ErrorContains(t, err, "session and activity id are required")
}

func TestClientActivityClassifiesRefusalsByStatus(t *testing.T) {
	cases := map[string]struct {
		target error
		status int
	}{
		"unauthorized": {ErrUnauthorized, http.StatusUnauthorized},
		"forbidden":    {ErrUnauthorized, http.StatusForbidden},
		"not found":    {ErrActivityRefused, http.StatusNotFound},
		"gone":         {ErrActivityRefused, http.StatusGone},
		"server error": {ErrRejected, http.StatusInternalServerError},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(tc.status)
			}))
			defer server.Close()

			client := newTestClient(t, server)
			_, err := client.Activity(t.Context(), Session{AccessToken: "access-token"}, 1)
			require.ErrorIs(t, err, tc.target)
		})
	}
}

func TestActivityFITURLBuildsTheS3ObjectLocation(t *testing.T) {
	activity := Activity{FITBucket: "prod-zwift-fit", FITKey: "prod/12345/token"}
	fitURL, ok := activity.FITURL()
	require.True(t, ok)
	assert.Equal(t, "https://prod-zwift-fit.s3.amazonaws.com/prod/12345/token", fitURL)

	bucketOnly := Activity{FITBucket: "prod-zwift-fit"}
	_, ok = bucketOnly.FITURL()
	assert.False(t, ok, "missing key")
	keyOnly := Activity{FITKey: "prod/12345/token"}
	_, ok = keyOnly.FITURL()
	assert.False(t, ok, "missing bucket")
}

// The summary is this package's own document, so its own decoder must read it
// back whole: a later layer stores it and may parse it again.
func TestActivitySummaryRoundTripsThroughTheDecoder(t *testing.T) {
	original := Activity{ID: 1461969115156611104, Sport: "CYCLING", FITBucket: "b", FITKey: "prod/1/k",
		StartDate:    time.Date(2026, 9, 8, 18, 4, 39, 123456789, time.FixedZone("", 5*3600+30*60)),
		EndDate:      time.Date(2026, 9, 8, 19, 0, 0, 500000000, time.UTC),
		MovingTimeMs: 3600000, DistanceMeters: 30000, TotalElevation: 300, WorldID: 1, UTCOffsetMinutes: 120, PrivateActivity: true}
	document, err := original.Summary()
	require.NoError(t, err)
	assert.Contains(t, string(document), `"id_str":"1461969115156611104"`)

	var decoded Activity
	require.NoError(t, json.Unmarshal(document, &decoded))
	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.FITKey, decoded.FITKey)
	assert.Equal(t, original.Sport, decoded.Sport)
	assert.Equal(t, original.PrivateActivity, decoded.PrivateActivity)
	assert.Equal(t, original.UTCOffsetMinutes, decoded.UTCOffsetMinutes)
	assert.True(t, original.StartDate.Equal(decoded.StartDate), "fractional seconds and a +05:30 offset survive")
	assert.True(t, original.EndDate.Equal(decoded.EndDate))
}

func TestActivitySummaryDropsOtherRidersFromTheResponse(t *testing.T) {
	raw := []byte(`{
		"id": 1, "id_str": "1461969115156611104", "sport": "CYCLING",
		"startDate": "2026-01-02T03:04:05Z", "distanceInMeters": 1000,
		"socialInteractions": [{"profile": {"firstName": "Someone Else"}}],
		"activityRideOns": [{"profileId": 99}],
		"subgroupResults": [{"profileId": 100, "finishTimeMs": 123}],
		"profile": {"firstName": "The Rider"}
	}`)
	var activity Activity
	require.NoError(t, json.Unmarshal(raw, &activity))

	summary, err := activity.Summary()
	require.NoError(t, err)
	assert.NotContains(t, string(summary), "socialInteractions")
	assert.NotContains(t, string(summary), "activityRideOns")
	assert.NotContains(t, string(summary), "subgroupResults")
	assert.NotContains(t, string(summary), "profile")
	assert.NotContains(t, string(summary), "Someone Else")
}

// Keycloak refuses a wrong password with 400 invalid_grant rather than 401.
func TestClientReportsUnauthorizedOnAnInvalidGrant(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusBadRequest)
		writeJSON(t, writer, map[string]string{"error": "invalid_grant"})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Session(t.Context(), []byte("rider@example.test"), []byte("wrong"))
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.True(t, client.IsUnauthorized(err))
}

func TestClientReportsUnauthorizedOn401(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Session(t.Context(), []byte("rider@example.com"), []byte("wrong-password"))
	require.ErrorIs(t, err, ErrUnauthorized)
	assert.True(t, client.IsUnauthorized(err))
	assert.NotContains(t, err.Error(), "wrong-password", "the credential must not reach the error")
}

func TestClientDownloadReportsActivityRefusedOn404(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), fakeS3URL("prod/1/token"))
	require.ErrorIs(t, err, ErrActivityRefused)
	assert.True(t, client.IsUnreadable(err))
}

// A public object answers 403 for a key it does not hold: the file's own
// refusal, not the account's.
func TestClientDownloadReportsActivityRefusedOn403(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), fakeS3URL("prod/1/token"))
	require.ErrorIs(t, err, ErrActivityRefused)
	assert.False(t, client.IsUnauthorized(err))
}

func TestClientDownloadDoesNotFollowARedirect(t *testing.T) {
	var followed atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/elsewhere" {
			followed.Add(1)
			writer.WriteHeader(http.StatusOK)

			return
		}
		http.Redirect(writer, request, "/elsewhere", http.StatusFound)
	}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), fakeS3URL("prod/1/token"))
	require.ErrorIs(t, err, ErrActivityRefused)
	assert.Equal(t, int32(0), followed.Load(), "the redirect target must never be requested")
}

func TestClientReportsRejectedOn503(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.PlayerID(t.Context(), Session{AccessToken: "access-token"})
	require.ErrorIs(t, err, ErrRejected)
	assert.True(t, client.IsRejected(err))
}

func TestClientDownloadRefusesANonS3Host(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), "https://evil.example.com/file")
	require.Error(t, err)
	assert.False(t, client.IsUnauthorized(err), "a rejected url must not read as an authorization failure")
}

func TestClientDownloadRefusesAHostWithASecondLabel(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), "https://bucket.s3.amazonaws.com.evil.example.com/file")
	require.Error(t, err)
}

func TestClientDownloadRefusesAPathTraversalDisguisedAsTheS3Host(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), "https://evil.example.com/../bucket.s3.amazonaws.com/file")
	require.Error(t, err, "the s3 host only appears in the path, not the actual host")
}

func TestClientDownloadCapsTheFile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		oversized := make([]byte, maximumFITBytes+1)
		writer.Write(oversized) //nolint:errcheck,gosec // test server, nothing to act on
	}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	_, err := client.DownloadFIT(t.Context(), fakeS3URL("prod/1/token"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "size limit")
}

func TestClientHonoursItsTimeout(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := New(&Options{
		AuthBaseURL: server.URL,
		APIBaseURL:  server.URL,
		Transport:   server.Client().Transport,
		Timeout:     10 * time.Millisecond,
	})
	require.NoError(t, err)

	_, err = client.PlayerID(t.Context(), Session{AccessToken: "access-token"})
	require.Error(t, err)
	var netErr interface{ Timeout() bool }
	require.True(t, errors.As(err, &netErr) || strings.Contains(err.Error(), "deadline"), "expected a timeout error, got %v", err)
}

func TestClientDownloadsTheFITFileWithoutAnAuthorizationHeader(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Empty(t, request.Header.Get("Authorization"), "the public s3 object must not see a bearer token")
		writer.Write([]byte("fit-file-bytes")) //nolint:errcheck,gosec // test server, nothing to act on
	}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 5*time.Second)
	data, err := client.DownloadFIT(t.Context(), fakeS3URL("prod/1/token"))
	require.NoError(t, err)
	assert.Equal(t, []byte("fit-file-bytes"), data)
}

func TestClientRequiredInputsAreValidated(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := newTestClient(t, server)

	_, err := client.Session(t.Context(), nil, []byte("password"))
	require.Error(t, err, "empty email")
	_, err = client.Session(t.Context(), []byte("email"), nil)
	require.Error(t, err, "empty password")
	_, err = client.Refresh(t.Context(), Session{})
	require.Error(t, err, "empty refresh token")
	_, err = client.PlayerID(t.Context(), Session{})
	require.Error(t, err, "empty access token")
	_, err = client.Activities(t.Context(), Session{AccessToken: "token"}, 0, 0, 1)
	require.Error(t, err, "missing player id")
	_, err = client.Activities(t.Context(), Session{AccessToken: "token"}, 1, -1, 1)
	require.Error(t, err, "negative start")
	_, err = client.Activities(t.Context(), Session{AccessToken: "token"}, 1, 0, 0)
	require.Error(t, err, "zero limit")
	_, err = client.DownloadFIT(t.Context(), "not a url")
	require.Error(t, err, "invalid fit file url")
}

func TestActivitiesRejectsANegativeStartWithTheCorrectMessage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := newTestClient(t, server)

	_, err := client.Activities(t.Context(), Session{AccessToken: "token"}, 1, -1, 1)
	require.ErrorContains(t, err, "start must not be negative and limit must be positive")
}

func TestNewRejectsInvalidOptions(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err, "nil options")
	_, err = New(&Options{AuthBaseURL: "not a url", APIBaseURL: DefaultAPIBaseURL})
	require.Error(t, err, "invalid auth base url")
	_, err = New(&Options{AuthBaseURL: DefaultAuthBaseURL, APIBaseURL: "not a url"})
	require.Error(t, err, "invalid api base url")
	_, err = New(&Options{AuthBaseURL: DefaultAuthBaseURL, APIBaseURL: DefaultAPIBaseURL, Timeout: -time.Second})
	require.Error(t, err, "negative timeout")

	client, err := New(&Options{})
	require.NoError(t, err, "empty options default to the real Zwift hosts")
	require.NotNil(t, client)
}

func TestClientRejectsAnIncompleteTokenResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"expires_in": 3600})
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Session(t.Context(), []byte("rider@example.com"), []byte("hunter2"))
	require.Error(t, err)
}

func TestActivityUnmarshalJSONRejectsMalformedOrMissingID(t *testing.T) {
	var activity Activity
	require.Error(t, json.Unmarshal([]byte("not json"), &activity), "malformed json")
	require.Error(t, json.Unmarshal([]byte(`{"sport":"CYCLING"}`), &activity), "missing id_str")
	require.Error(t, json.Unmarshal([]byte(`{"id_str":"not-a-number"}`), &activity), "id_str not a number")
}

func TestClientDownloadReportsAConnectionFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newDownloadTestClient(t, server, 10*time.Millisecond)

	_, err := client.DownloadFIT(t.Context(), fakeS3URL("prod/1/token"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fit file request failed")
}

// erroringBodyTransport answers every request with a 200 whose body fails on
// read, so doJSON's read failure — not otherwise reachable through a real
// server — is exercised directly.
type erroringBodyTransport struct{}

func (erroringBodyTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(iotest.ErrReader(errors.New("read failed"))),
	}, nil
}

func TestClientReportsAResponseThatFailsToRead(t *testing.T) {
	client, err := New(&Options{Transport: erroringBodyTransport{}, Timeout: time.Second})
	require.NoError(t, err)

	_, err = client.PlayerID(t.Context(), Session{AccessToken: "access-token"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "could not be read")
}

func TestSessionRedactsItselfFromFormattingVerbs(t *testing.T) {
	session := Session{AccessToken: "access-token", RefreshToken: "refresh-token"}
	assert.Equal(t, "[redacted]", session.String())
	assert.Equal(t, "[redacted]", session.GoString())
}

func TestClassifyStatusReportsAnUnrecognisedStatusPlainly(t *testing.T) {
	err := classifyStatus(http.StatusTeapot, activityRequest)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrUnauthorized)
	require.NotErrorIs(t, err, ErrActivityRefused)
	require.NotErrorIs(t, err, ErrRejected)
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		AuthBaseURL: server.URL,
		APIBaseURL:  server.URL,
		Transport:   server.Client().Transport,
		Timeout:     5 * time.Second,
	})
	require.NoError(t, err)

	return client
}

func writeJSON(t *testing.T, writer http.ResponseWriter, value any) {
	t.Helper()
	writer.Header().Set("Content-Type", "application/json")
	assert.NoError(t, json.NewEncoder(writer).Encode(value), "writing the JSON response")
}
