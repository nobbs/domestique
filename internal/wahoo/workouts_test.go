package wahoo

import (
	"bytes"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
			"id":                    42,
			"file":                  map[string]string{"url": "https://cdn.wahooligan.com/workouts/42.fit"},
			"distance_accum":        "1234.5",
			"duration_active_accum": "3600.0",
			"duration_total_accum":  3900,
			"ascent_accum":          "120.25",
		}})
	}))
	defer server.Close()

	summary, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.wahooligan.com/workouts/42.fit", summary.FileURL)
	assert.InDelta(t, 1234.5, summary.DistanceMetres, 1e-9)
	assert.InDelta(t, 3600.0, summary.ActiveSeconds, 1e-9)
	assert.InDelta(t, 3900.0, summary.TotalSeconds, 1e-9)
	assert.InDelta(t, 120.25, summary.AscentMetres, 1e-9)
	assert.Contains(t, string(summary.Raw), `"distance_accum":"1234.5"`)
}

// Wahoo sends the totals as strings holding decimals; a missing one is zero and
// a value that is not a number at all is refused rather than read as zero.
func TestClientReadsWorkoutSummaryTotalsWahooSendsAsStrings(t *testing.T) {
	cases := map[string]struct {
		summary map[string]any
		wantErr string
		want    float64
	}{
		"missing":     {summary: map[string]any{}, want: 0},
		"null":        {summary: map[string]any{"distance_accum": nil}, want: 0},
		"empty":       {summary: map[string]any{"distance_accum": ""}, want: 0},
		"number":      {summary: map[string]any{"distance_accum": 12.5}, want: 12.5},
		"non-numeric": {summary: map[string]any{"distance_accum": "later"}, wantErr: "totals were not numbers"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			body := map[string]any{"file": map[string]string{"url": "https://cdn.wahooligan.com/workouts/42.fit"}}
			maps.Copy(body, tc.summary)
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, map[string]any{"workout_summary": body})
			}))
			defer server.Close()

			summary, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)

				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tc.want, summary.DistanceMetres, 1e-9)
		})
	}
}

// The listing carries when each ride started, which is what decides the order
// summaries are read in.
func TestClientReadsWhenAWorkoutStarted(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{
			"workouts": []map[string]any{{"id": 1, "starts": "2026-04-01T06:30:00Z"}},
			"total":    1, "page": 1, "per_page": 100,
		})
	}))
	defer server.Close()

	workouts, err := newTestClient(t, server).ListWorkouts(t.Context(), "access-token")
	require.NoError(t, err)
	require.Len(t, workouts, 1)
	assert.Equal(t, time.Date(2026, 4, 1, 6, 30, 0, 0, time.UTC), workouts[0].Starts.UTC())
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

func TestDecimalResetsOnNull(t *testing.T) {
	value := decimal(12)
	require.NoError(t, value.UnmarshalJSON([]byte("null")))
	assert.Zero(t, value)
	require.NoError(t, value.UnmarshalJSON([]byte(`"3.5"`)))
	assert.InDelta(t, 3.5, float64(value), 0)
	require.NoError(t, value.UnmarshalJSON([]byte(`""`)))
	assert.Zero(t, value)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
