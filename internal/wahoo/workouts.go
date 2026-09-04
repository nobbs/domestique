package wahoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const (
	workoutPageSize        = 100
	maximumWorkouts        = 10_000
	maximumWorkoutFITBytes = 16 << 20
)

// Workout is the list-level metadata Wahoo returns for one recorded activity.
type Workout struct {
	ID int64 `json:"id"`
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	WorkoutTypeID int `json:"workout_type_id"`
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	WorkoutTypeLocationID int `json:"workout_type_location_id"`
}

// WorkoutSummary contains Wahoo's original summary and the URL of its FIT file.
type WorkoutSummary struct {
	FileURL string
	Raw     json.RawMessage
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

	var envelope struct {
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		WorkoutSummary json.RawMessage `json:"workout_summary"`
	}
	if err := c.doJSON(request, &envelope); err != nil {
		return WorkoutSummary{}, err
	}
	if len(envelope.WorkoutSummary) == 0 || string(envelope.WorkoutSummary) == "null" {
		return WorkoutSummary{}, errors.New("wahoo: workout summary was missing")
	}
	var summary struct {
		File struct {
			URL string `json:"url"`
		} `json:"file"`
	}
	if err := json.Unmarshal(envelope.WorkoutSummary, &summary); err != nil {
		return WorkoutSummary{}, errors.New("wahoo: workout summary was not valid json")
	}
	if summary.File.URL == "" {
		return WorkoutSummary{}, errors.New("wahoo: workout summary did not contain a file url")
	}

	return WorkoutSummary{Raw: envelope.WorkoutSummary, FileURL: summary.File.URL}, nil
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
