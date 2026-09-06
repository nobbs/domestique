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

// The head is one request: it reads the first page and the account's own total
// without walking the pages behind it.
func TestClientReadsTheWorkoutListingHeadInOneRequest(t *testing.T) {
	var requests int
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		assert.Equal(t, "1", request.URL.Query().Get("page"))
		writeJSON(t, writer, map[string]any{
			"workouts": []map[string]int{{"id": 1, "workout_type_id": 15, "workout_type_location_id": 1}},
			"total":    7, "page": 1, "per_page": 100,
		})
	}))
	defer server.Close()

	workouts, total, err := newTestClient(t, server).WorkoutListingHead(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, []Workout{{ID: 1, WorkoutTypeID: 15, WorkoutTypeLocationID: 1}}, workouts)
	assert.Equal(t, 7, total)
	assert.Equal(t, 1, requests)
}

// An empty first page the account says holds workouts is a broken listing, and
// costs one request to find out rather than a walk that fails at its end.
func TestClientReportsAWorkoutListingHeadThatEndedEarly(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"workouts": []any{}, "total": 3, "page": 1, "per_page": 100})
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).WorkoutListingHead(t.Context(), "access-token")
	require.ErrorContains(t, err, "ended before its total")
}

func TestClientRefusesAWorkoutListingHeadWithoutAToken(t *testing.T) {
	_, _, err := (&Client{}).WorkoutListingHead(t.Context(), "")
	require.ErrorContains(t, err, "access token is required")
}

func TestClientReportsAnInvalidWorkoutListingHead(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"workouts": []any{}, "total": 1, "page": 2, "per_page": 100})
	}))
	defer server.Close()

	_, _, err := newTestClient(t, server).WorkoutListingHead(t.Context(), "access-token")
	require.ErrorContains(t, err, "pagination was invalid")
}

func TestClientReadsTheRawWorkoutSummary(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodGet, request.Method)
		assert.Equal(t, "Bearer access-token", request.Header.Get("Authorization"))
		assert.Equal(t, "/v1/workouts/42/workout_summary", request.URL.Path)
		writeJSON(t, writer, map[string]any{
			"id":                    42,
			"file":                  map[string]string{"url": "https://cdn.wahooligan.com/workouts/42.fit"},
			"distance_accum":        "1234.5",
			"duration_active_accum": "3600.0",
			"duration_total_accum":  3900,
			"ascent_accum":          "120.25",
		})
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
			body := map[string]any{
				"id":   42,
				"file": map[string]string{"url": "https://cdn.wahooligan.com/workouts/42.fit"},
			}
			maps.Copy(body, tc.summary)
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, body)
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

// A manually entered ride has totals but no FIT file, and its summary is still
// the only place those totals come from.
func TestClientReadsAWorkoutSummaryWithoutAFile(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"id": 42, "distance_accum": "1234.5", "manual": true})
	}))
	defer server.Close()

	summary, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
	require.NoError(t, err)
	assert.Empty(t, summary.FileURL)
	assert.InDelta(t, 1234.5, summary.DistanceMetres, 1e-9)
}

func TestClientRejectsInvalidWorkoutPagination(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(t, writer, map[string]any{"workouts": []any{}, "total": 1, "page": 1, "per_page": 100})
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListWorkouts(t.Context(), "access-token")
	require.ErrorContains(t, err, "ended before its total")
}

// The summary endpoint answers with the summary itself. A body wrapped in a
// "workout_summary" key is the listing's shape, not this one's, and reading it
// as though it were carried nothing out of any real account.
func TestClientReportsAMissingWorkoutSummary(t *testing.T) {
	cases := map[string]any{
		"null body":              nil,
		"object without its own": map[string]any{"file": map[string]string{"url": "https://cdn.wahooligan.com/42.fit"}},
		"listing envelope":       map[string]any{"workout_summary": map[string]any{"id": 42}},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, body)
			}))
			defer server.Close()

			_, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
			require.ErrorContains(t, err, "unreadable: missing")
		})
	}
}

// A rejection that belongs to one workout is told apart from one that belongs
// to the provider, so a poll knows which it may step past.
func TestClientTellsAnUnreadableWorkoutFromAFailedProvider(t *testing.T) {
	cases := map[string]struct {
		body       any
		status     int
		unreadable bool
		rejected   bool
	}{
		// Wahoo refuses one workout's summary and serves the next on the same
		// token, so a refusal is the workout's own (#487).
		"refused":      {status: http.StatusUnauthorized, unreadable: true},
		"missing":      {status: http.StatusNotFound, unreadable: true},
		"no summary":   {status: http.StatusOK, body: nil, unreadable: true},
		"wrong shape":  {status: http.StatusOK, body: "x", unreadable: true},
		"provider":     {status: http.StatusInternalServerError},
		"rate limited": {status: http.StatusTooManyRequests},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if tc.status != http.StatusOK {
					writer.WriteHeader(tc.status)

					return
				}
				writeJSON(t, writer, tc.body)
			}))
			defer server.Close()

			_, err := newTestClient(t, server).WorkoutSummary(t.Context(), "access-token", 42)
			require.Error(t, err)
			assert.Equal(t, tc.unreadable, errors.Is(err, ErrWorkoutUnreadable), "%v", err)
			assert.Equal(t, tc.rejected, errors.Is(err, ErrRequestRejected), "%v", err)
			// #455 stands: only the token endpoint concludes a grant is spent.
			require.NotErrorIs(t, err, ErrUnauthorized, "a data request judged the refresh token")
			assert.NotContains(t, err.Error(), "access-token")
		})
	}
}

// Only a summary read may answer for one workout; a listing that is refused or
// missing says something about the account, and is never a skip.
func TestClientNeverMarksAWorkoutListingUnreadable(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := newTestClient(t, server).ListWorkouts(t.Context(), "access-token")
	require.ErrorContains(t, err, "HTTP 404")
	require.NotErrorIs(t, err, ErrWorkoutUnreadable)
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
				writeJSON(t, writer, "x")
			},
			read: func(c *Client) error { _, err := c.WorkoutSummary(t.Context(), "access-token", 42); return err },
			want: "unreadable: not the shape expected",
		},
		"summary whose file is not an object": {
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writeJSON(t, writer, map[string]any{"id": 42, "file": "https://cdn.wahooligan.com/42.fit"})
			},
			read: func(c *Client) error { _, err := c.WorkoutSummary(t.Context(), "access-token", 42); return err },
			want: "unreadable: not the shape expected",
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
		refused bool
	}{
		"transport failure": {
			respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial failed") },
			want:    "request failed: dial failed",
		},
		"refused by the CDN": {
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusNotFound, Header: make(http.Header), Body: http.NoBody}, nil
			},
			want:    "was refused: HTTP 404",
			refused: true,
		},
		"rate limited by the CDN": {
			respond: func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: http.StatusTooManyRequests, Header: make(http.Header), Body: http.NoBody}, nil
			},
			want: "returned HTTP 429",
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
			if tc.refused {
				assert.ErrorIs(t, err, ErrWorkoutFileRefused)
			} else {
				assert.NotErrorIs(t, err, ErrWorkoutFileRefused)
			}
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

// A listing entry carries the same summary document the sub-resource answers
// with, so a poll that reads it from the listing stores exactly what a request
// for it would have; an entry without one, or with one that is not a summary,
// carries nothing and leaves the poll to ask for it.
func TestClientReadsTheSummaryAListingCarries(t *testing.T) {
	document := map[string]any{
		"id":                    42,
		"file":                  map[string]string{"url": "https://cdn.wahooligan.com/workouts/42.fit"},
		"distance_accum":        "1234.5",
		"duration_active_accum": "3600.0",
		"duration_total_accum":  3900,
		"ascent_accum":          "120.25",
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/workouts/42/workout_summary" {
			writeJSON(t, writer, document)

			return
		}
		writeJSON(t, writer, map[string]any{
			"workouts": []map[string]any{
				{"id": 42, "workout_type_id": 15, "workout_summary": document},
				{"id": 43, "workout_type_id": 15, "workout_summary": nil},
				{"id": 44, "workout_type_id": 15},
				{"id": 45, "workout_type_id": 15, "workout_summary": map[string]any{"file": nil}},
				{"id": 46, "workout_type_id": 15, "workout_summary": map[string]any{
					"id": 46, "file": map[string]string{"url": "https://cdn.wahooligan.com/workouts/46.fit"},
					"distance_accum": "1", "duration_active_accum": "2", "duration_total_accum": "3",
				}},
				{"id": 47, "workout_type_id": 15, "workout_summary": map[string]any{
					"id": 47, "file": map[string]string{"url": "https://cdn.wahooligan.com/workouts/47.fit"},
					"distance_accum": "1", "duration_active_accum": "2", "duration_total_accum": "3", "ascent_accum": nil,
				}},
			},
			"total": 6, "page": 1, "per_page": 100,
		})
	}))
	defer server.Close()
	client := newTestClient(t, server)

	workouts, err := client.ListWorkouts(t.Context(), "access-token")
	require.NoError(t, err)
	require.Len(t, workouts, 6)
	require.NotNil(t, workouts[0].Summary, "the listing's summary was dropped")
	requested, err := client.WorkoutSummary(t.Context(), "access-token", 42)
	require.NoError(t, err)
	assert.Equal(t, requested, *workouts[0].Summary, "the listing's summary differs from the sub-resource's")
	assert.JSONEq(t, string(requested.Raw), string(workouts[0].Summary.Raw))
	assert.Nil(t, workouts[1].Summary, "a null summary was read as one")
	assert.Nil(t, workouts[2].Summary, "an absent summary was read as one")
	assert.Nil(t, workouts[3].Summary, "an incomplete summary was read as one")
	assert.Nil(t, workouts[4].Summary, "a summary missing one total was read as one")
	// A null total is Wahoo's zero, as the sub-resource reads it (see
	// TestClientReadsWorkoutSummaryTotalsWahooSendsAsStrings).
	require.NotNil(t, workouts[5].Summary, "a summary with a null total was refused")
	assert.Zero(t, workouts[5].Summary.AscentMetres)
}
