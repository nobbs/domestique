package wahoo

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientListsEveryWorkoutPage(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"))
		assert.Equal(t, "100", request.URL.Query().Get("per_page"))
		switch request.URL.Query().Get("page") {
		case "1":
			writeJSON(t, writer, map[string]any{
				"workouts": []map[string]int{{"id": 1, "workout_type_id": 15, "workout_type_location_id": 1}},
				"total":    2, "page": 1, "per_page": 100,
			})
		case "2":
			writeJSON(t, writer, map[string]any{
				"workouts": []map[string]int{{"id": 2, "workout_type_id": 61, "workout_type_location_id": 0}},
				"total":    2, "page": 2, "per_page": 100,
			})
		default:
			assert.Failf(t, "unexpected page", "%q", request.URL.Query().Get("page"))
		}
	}))
	defer server.Close()

	workouts, err := newTestClient(t, server).ListWorkouts(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, []Workout{
		{ID: 1, WorkoutTypeID: 15, WorkoutTypeLocationID: 1},
		{ID: 2, WorkoutTypeID: 61, WorkoutTypeLocationID: 0},
	}, workouts)
}

func TestClientReadsTheRawWorkoutSummary(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"))
		assert.Equal(t, "/v1/workouts/42/workout_summary", request.URL.Path)
		writeJSON(t, writer, map[string]any{"workout_summary": map[string]any{
			"id":   42,
			"file": map[string]string{"url": "https://cdn.wahooligan.com/workouts/42.fit"},
		}})
	}))
	defer server.Close()

	summary, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.wahooligan.com/workouts/42.fit", summary.FileURL)
	assert.JSONEq(t, `{"id":42,"file":{"url":"https://cdn.wahooligan.com/workouts/42.fit"}}`, string(summary.Raw))
}

func TestClientRejectsInvalidWorkoutPagination(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"workouts": []any{}, "total": 1, "page": 1, "per_page": 100})
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListWorkouts(t.Context(), "access-token")
	require.ErrorContains(t, err, "ended before its total")
}

func TestClientReportsAMissingWorkoutSummary(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"workout_summary": nil})
	}))
	defer server.Close()

	_, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
	require.ErrorContains(t, err, "summary was missing")
}

func TestClientRejectsAWorkoutListingBeyondItsBounds(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"workouts": []any{}, "total": maximumWorkouts + 1, "page": 1, "per_page": 100})
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListWorkouts(t.Context(), "access-token")
	require.ErrorContains(t, err, "exceeded configured bounds")
}

func TestClientDownloadsWorkoutFITWithoutCredentials(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()

	client := newTestClient(t, server)
	client.client.Transport = roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "cdn.wahooligan.com", request.URL.Host)
		assert.Empty(t, request.Header.Get("Authorization"))

		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader([]byte("fit"))),
			Header:     make(http.Header),
		}, nil
	})

	fit, err := client.DownloadWorkoutFIT(t.Context(), "https://cdn.wahooligan.com/workouts/42.fit")
	require.NoError(t, err)
	assert.Equal(t, []byte("fit"), fit)
}

func TestClientRejectsWorkoutFITURLOutsideTheWahooCDN(t *testing.T) {
	server := httptest.NewTLSServer(http.NotFoundHandler())
	defer server.Close()

	client := newTestClient(t, server)

	_, err := client.DownloadWorkoutFIT(t.Context(), "https://example.test/workouts/42.fit")
	require.ErrorContains(t, err, "url was invalid")
}

func TestClientRejectsWorkoutReadsItCannotTrust(t *testing.T) {
	cases := map[string]struct {
		handler http.HandlerFunc
		read    func(*Client) error
		want    string
	}{
		"listing without a token": {
			read: func(c *Client) error { _, err := c.ListWorkouts(t.Context(), ""); return err },
			want: "access token is required",
		},
		"listing that fails upstream": {
			handler: func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusInternalServerError) },
			read:    func(c *Client) error { _, err := c.ListWorkouts(t.Context(), "access-token"); return err },
			want:    "HTTP 500",
		},
		"listing that answers the wrong page": {
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, map[string]any{"workouts": []any{}, "total": 0, "page": 2, "per_page": 100})
			},
			read: func(c *Client) error { _, err := c.ListWorkouts(t.Context(), "access-token"); return err },
			want: "pagination was invalid",
		},
		"summary without a token or id": {
			read: func(c *Client) error { _, err := c.WorkoutSummary(t.Context(), "access-token", 0); return err },
			want: "access token and workout id are required",
		},
		"summary that fails upstream": {
			handler: func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusInternalServerError) },
			read:    func(c *Client) error { _, err := c.WorkoutSummary(t.Context(), "access-token", 42); return err },
			want:    "HTTP 500",
		},
		"summary that is not an object": {
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, map[string]any{"workout_summary": "x"})
			},
			read: func(c *Client) error { _, err := c.WorkoutSummary(t.Context(), "access-token", 42); return err },
			want: "summary was not valid json",
		},
		"summary without a file": {
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, map[string]any{"workout_summary": map[string]any{"id": 42}})
			},
			read: func(c *Client) error { _, err := c.WorkoutSummary(t.Context(), "access-token", 42); return err },
			want: "did not contain a file url",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			handler := tc.handler
			if handler == nil {
				handler = func(_ http.ResponseWriter, _ *http.Request) { assert.Fail(t, "no request expected") }
			}
			server := httptest.NewTLSServer(handler)
			defer server.Close()

			require.ErrorContains(t, tc.read(newTestClient(t, server)), tc.want)
		})
	}
}

func TestClientRejectsWorkoutFITDownloadsItCannotTrust(t *testing.T) {
	cases := map[string]struct {
		respond func(*http.Request) (*http.Response, error)
		want    string
	}{
		"transport failure": {
			respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") },
			want:    "request failed: dial failed",
		},
		"redirect": {
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusFound, Header: http.Header{"Location": {"https://example.test/"}}, Body: http.NoBody}, nil
			},
			want: "returned HTTP 302",
		},
		"empty body": {
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: http.NoBody}, nil
			},
			want: "empty or exceeded size limit",
		},
		"oversized body": {
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(strings.Repeat("f", maximumWorkoutFITBytes+1)))}, nil
			},
			want: "empty or exceeded size limit",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.NotFoundHandler())
			defer server.Close()
			client := newTestClient(t, server)
			client.client.Transport = roundTripperFunc(tc.respond)

			_, err := client.DownloadWorkoutFIT(t.Context(), "https://cdn.wahooligan.com/workouts/42.fit")
			require.ErrorContains(t, err, tc.want)
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
