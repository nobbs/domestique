package zwift

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestClientReportsActivityRefusedOn404(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.Activity(t.Context(), Session{AccessToken: "access-token"}, 1)
	require.ErrorIs(t, err, ErrActivityRefused)
	assert.True(t, client.IsUnreadable(err))
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

func TestClientDownloadRefusesAForeignHost(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	_, err := client.DownloadFIT(t.Context(), Session{AccessToken: "access-token"}, "https://evil.example.com/file")
	require.Error(t, err)
	assert.False(t, client.IsUnauthorized(err), "a rejected url must not read as an authorization failure")
}

func TestClientDownloadCapsTheFile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusOK)
		oversized := make([]byte, maximumFITBytes+1)
		writer.Write(oversized) //nolint:errcheck,gosec // test server, nothing to act on
	}))
	defer server.Close()

	client := newTestClient(t, server)
	fileURL := server.URL + "/api/activities/1/file/1"
	_, err := client.DownloadFIT(t.Context(), Session{AccessToken: "access-token"}, fileURL)
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

func TestClientReadsOneActivity(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/activities/1461969115156611104", request.URL.Path)
		body := `{
			"id": 1461969115156611000, "id_str": "1461969115156611104", "sport": "CYCLING",
			"fitnessData": {"status": "AVAILABLE", "fullDataUrl": "` + server.URL + `/api/activities/1461969115156611104/file/1"}
		}`
		writer.Write([]byte(body)) //nolint:errcheck,gosec // test server, nothing to act on
	}))
	defer server.Close()

	client := newTestClient(t, server)
	one, err := client.Activity(t.Context(), Session{AccessToken: "access-token"}, 1461969115156611104)
	require.NoError(t, err)
	assert.Equal(t, int64(1461969115156611104), one.ID)
	assert.Equal(t, "AVAILABLE", one.FitnessStatus)
	assert.NotEmpty(t, one.FullDataURL)
}

func TestClientDownloadsTheFITFile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"), "authorization")
		writer.Write([]byte("fit-file-bytes")) //nolint:errcheck,gosec // test server, nothing to act on
	}))
	defer server.Close()

	client := newTestClient(t, server)
	data, err := client.DownloadFIT(t.Context(), Session{AccessToken: "access-token"}, server.URL+"/api/activities/1/file/1")
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
	_, err = client.Activity(t.Context(), Session{AccessToken: "token"}, 0)
	require.Error(t, err, "missing activity id")
	_, err = client.DownloadFIT(t.Context(), Session{}, server.URL+"/x")
	require.Error(t, err, "empty access token")
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

	client, err := New(&Options{
		AuthBaseURL: server.URL,
		APIBaseURL:  server.URL,
		Transport:   server.Client().Transport,
		Timeout:     10 * time.Millisecond,
	})
	require.NoError(t, err)

	_, err = client.DownloadFIT(t.Context(), Session{AccessToken: "access-token"}, server.URL+"/api/activities/1/file/1")
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
	err := classifyStatus(http.StatusTeapot, true)
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
