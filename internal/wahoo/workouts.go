package wahoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	workoutPageSize        = 100
	maximumWorkouts        = 10_000
	maximumWorkoutFITBytes = 16 << 20
)

// Workout is the list-level metadata Wahoo returns for one recorded activity.
type Workout struct {
	// Starts is when the rider began recording, in Wahoo's own RFC 3339 form.
	Starts time.Time `json:"starts"`
	ID     int64     `json:"id"`
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	WorkoutTypeID int `json:"workout_type_id"`
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	WorkoutTypeLocationID int `json:"workout_type_location_id"`
}

// WorkoutSummary contains Wahoo's original summary, the URL of its FIT file,
// and the totals decoded from it.
type WorkoutSummary struct {
	// FileURL is empty for a summary Wahoo holds no FIT file for, which a
	// manually entered ride never has.
	FileURL        string
	Raw            json.RawMessage
	DistanceMetres float64
	ActiveSeconds  float64
	TotalSeconds   float64
	AscentMetres   float64
}

// ErrWorkoutUnreadable reports a summary rejection that belongs to that one
// workout — Wahoo refuses or cannot find it, or answers something that is not a
// summary — rather than to the connection, the quota or the grant.
var ErrWorkoutUnreadable = errors.New("wahoo: workout summary was unreadable")

// decimal is one of Wahoo's accumulated totals, which it encodes as a JSON
// string holding a decimal rather than as a number.
type decimal float64

func (d *decimal) UnmarshalJSON(raw []byte) error {
	*d = 0
	if bytes.Equal(raw, []byte("null")) {
		return nil
	}
	text := string(raw)
	if quoted, err := strconv.Unquote(text); err == nil {
		text = quoted
	}
	if text == "" {
		return nil
	}
	value, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return fmt.Errorf("wahoo: workout summary total is not a number: %w", err)
	}
	*d = decimal(value)

	return nil
}

// ListWorkouts returns every workout in Wahoo's paginated list.
//
// Account selection remains deliberately outside this adapter until the service
// has a user model; this client only reads the account it is given a token for.
func (c *Client) ListWorkouts(ctx context.Context, accessToken string) ([]Workout, error) {
	if accessToken == "" {
		return nil, errors.New("wahoo: access token is required")
	}

	var workouts []Workout
	for page := 1; ; page++ {
		endpoint := c.endpoint(c.apiBaseURL, "/v1/workouts")
		endpoint.RawQuery = url.Values{
			"page":     {fmt.Sprint(page)},
			"per_page": {fmt.Sprint(workoutPageSize)},
		}.Encode()
		request, err := c.newRequest(ctx, http.MethodGet, endpoint, http.NoBody, accessToken)
		if err != nil {
			return nil, err
		}

		var response workoutPage
		if err := c.doJSON(request, &response); err != nil {
			return nil, err
		}
		if response.Page != page || response.PerPage <= 0 || response.Total < len(workouts)+len(response.Workouts) {
			return nil, errors.New("wahoo: workout listing pagination was invalid")
		}
		if response.Total > maximumWorkouts {
			return nil, errors.New("wahoo: workout listing exceeded configured bounds")
		}
		workouts = append(workouts, response.Workouts...)
		if len(workouts) >= response.Total {
			return workouts, nil
		}
		if len(response.Workouts) == 0 {
			return nil, errors.New("wahoo: workout listing ended before its total")
		}
	}
}

// WorkoutSummary returns the original Wahoo summary and its FIT file URL.
//
// The summary is the response body itself. Only a workout listing wraps one in
// a "workout_summary" key; this endpoint does not, and reading it as though it
// did fails every summary in an account rather than none.
func (c *Client) WorkoutSummary(ctx context.Context, accessToken string, workoutID int64) (WorkoutSummary, error) {
	if accessToken == "" || workoutID <= 0 {
		return WorkoutSummary{}, errors.New("wahoo: access token and workout id are required")
	}

	request, err := c.newRequest(
		ctx,
		http.MethodGet,
		c.endpoint(c.apiBaseURL, fmt.Sprintf("/v1/workouts/%d/workout_summary", workoutID)),
		http.NoBody,
		accessToken,
	)
	if err != nil {
		return WorkoutSummary{}, err
	}

	var body json.RawMessage
	if err := c.doJSON(request, &body); err != nil {
		var status *statusError
		if errors.As(err, &status) &&
			(status.status == http.StatusUnauthorized || status.status == http.StatusNotFound) {
			return WorkoutSummary{}, fmt.Errorf("%w: HTTP %d", ErrWorkoutUnreadable, status.status)
		}

		return WorkoutSummary{}, err
	}
	var summary struct {
		File struct {
			URL string `json:"url"`
		} `json:"file"`
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		DistanceAccum decimal `json:"distance_accum"`
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		DurationActiveAccum decimal `json:"duration_active_accum"`
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		DurationTotalAccum decimal `json:"duration_total_accum"`
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		AscentAccum decimal `json:"ascent_accum"`
		ID          int64   `json:"id"`
	}
	if err := json.Unmarshal(body, &summary); err != nil {
		var numErr *strconv.NumError
		if errors.As(err, &numErr) { //nolint:modernize // errors.As is unambiguous to every tool reviewing this code.
			return WorkoutSummary{}, fmt.Errorf("wahoo: workout summary totals were not numbers: %w", ErrWorkoutUnreadable)
		}

		// Syntax cannot be what failed: doJSON decoded this body into a
		// json.RawMessage, which validates the whole document first.
		return WorkoutSummary{}, fmt.Errorf("wahoo: workout summary was not the shape expected: %w", ErrWorkoutUnreadable)
	}
	// A workout Wahoo holds no summary for decodes to a zero value, whether it
	// answers with a null body or an object carrying nothing of its own.
	if summary.ID <= 0 {
		return WorkoutSummary{}, fmt.Errorf("wahoo: workout summary was missing: %w", ErrWorkoutUnreadable)
	}
	return WorkoutSummary{
		Raw:            body,
		FileURL:        summary.File.URL,
		DistanceMetres: float64(summary.DistanceAccum),
		ActiveSeconds:  float64(summary.DurationActiveAccum),
		TotalSeconds:   float64(summary.DurationTotalAccum),
		AscentMetres:   float64(summary.AscentAccum),
	}, nil
}

// DownloadWorkoutFIT reads the FIT file Wahoo's workout summary names.
func (c *Client) DownloadWorkoutFIT(ctx context.Context, fileURL string) (data []byte, err error) {
	parsed, parseErr := url.Parse(fileURL)
	if parseErr != nil || parsed.Scheme != "https" || parsed.Host != "cdn.wahooligan.com" ||
		parsed.User != nil || parsed.Fragment != "" {
		return nil, errors.New("wahoo: workout file url was invalid")
	}

	// Wahoo puts activity FIT files on its CDN, outside the API quota and without
	// a bearer token; redirects are refused so a summary cannot turn into SSRF.
	download := &http.Client{
		Timeout:   c.client.Timeout,
		Transport: c.client.Transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), http.NoBody)
	if requestErr != nil {
		// Not wrapped: a *url.Error message would carry the signed CDN URL.
		return nil, errors.New("wahoo: workout file request could not be created")
	}
	response, err := download.Do(request)
	if err != nil {
		var urlErr *url.Error
		if errors.As(err, &urlErr) { //nolint:modernize // errors.As is unambiguous to every tool reviewing this code.
			err = urlErr.Err
		}

		return nil, fmt.Errorf("wahoo: workout file request failed: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("wahoo: workout file request returned HTTP %d", response.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, maximumWorkoutFITBytes+1))
	if err != nil {
		return nil, errors.New("wahoo: workout file could not be read")
	}
	if len(data) == 0 || len(data) > maximumWorkoutFITBytes {
		return nil, errors.New("wahoo: workout file was empty or exceeded size limit")
	}

	return data, nil
}

type workoutPage struct {
	Workouts []Workout `json:"workouts"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	PerPage int `json:"per_page"`
}
