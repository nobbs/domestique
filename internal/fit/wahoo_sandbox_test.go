//go:build wahoo_sandbox

package fit_test

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fitadapter "github.com/nobbs/domestique/internal/fit"
	"github.com/nobbs/domestique/internal/route"
)

const (
	wahooSandboxEnvironment = "sandbox"
	wahooSandboxBaseURL     = "https://api.wahooligan.com"
	wahooResponseLimit      = 1 << 20
)

func TestWahooSandboxAcceptance(t *testing.T) {
	require.Equalf(t, wahooSandboxEnvironment, os.Getenv("DOMESTIQUE_WAHOO_ENVIRONMENT"),
		"DOMESTIQUE_WAHOO_ENVIRONMENT must be %q for this test", wahooSandboxEnvironment)
	accessToken := os.Getenv("DOMESTIQUE_WAHOO_SANDBOX_ACCESS_TOKEN")
	require.NotEmpty(t, accessToken, "DOMESTIQUE_WAHOO_SANDBOX_ACCESS_TOKEN is required")
	baseURL, err := parseSandboxBaseURL(os.Getenv("DOMESTIQUE_WAHOO_SANDBOX_BASE_URL"))
	require.NoError(t, err, "sandbox base url")

	stage := sandboxStage(t)
	encoded, err := fitadapter.New().Encode(t.Context(), stage)
	require.NoError(t, err)
	externalID, err := sandboxExternalID()
	require.NoError(t, err, "creating external id")

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 30 * time.Second}
	created, err := createRoute(ctx, client, baseURL, accessToken, &stage, encoded, externalID)
	require.NoError(t, err, "creating sandbox route")
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		assert.NoError(t, deleteRoute(cleanupCtx, client, baseURL, accessToken, created.ID),
			"deleting temporary sandbox route")
	})

	fetched, err := getRoute(ctx, client, baseURL, accessToken, created.ID)
	require.NoError(t, err, "retrieving created sandbox route")
	assert.Equal(t, created.ID, fetched.ID, "wahoo returned a different route")
	assert.Equal(t, externalID, fetched.ExternalID, "wahoo did not retain the external id")
	assert.NotEmpty(t, fetched.File.URL, "wahoo retained no file for the uploaded course")
}

type wahooRoute struct {
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	ExternalID string `json:"external_id"`
	File       struct {
		URL string `json:"url"`
	} `json:"file"`
	ID int64 `json:"id"`
}

func createRoute(
	ctx context.Context,
	client *http.Client,
	baseURL *url.URL,
	accessToken string,
	stage *route.Stage,
	encoded []byte,
	externalID string,
) (wahooRoute, error) {
	geometry := stage.Geometry()
	form := url.Values{
		"route[file]":                   {"data:application/vnd.fit;base64," + base64.StdEncoding.EncodeToString(encoded)},
		"route[filename]":               {"domestique-acceptance.fit"},
		"route[external_id]":            {externalID},
		"route[provider_updated_at]":    {time.Now().UTC().Format(time.RFC3339Nano)},
		"route[name]":                   {stage.Title()},
		"route[workout_type_family_id]": {"0"},
		"route[start_lat]":              {fmt.Sprintf("%.7f", geometry[0].Latitude)},
		"route[start_lng]":              {fmt.Sprintf("%.7f", geometry[0].Longitude)},
		"route[distance]":               {"100"},
		"route[ascent]":                 {"0"},
	}

	//nolint:gosec // The sandbox-only base URL is parsed as an HTTPS origin.
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, routeURL(baseURL, "/v1/routes"), strings.NewReader(form.Encode()))
	if err != nil {
		return wahooRoute{}, fmt.Errorf("creating sandbox route request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var created wahooRoute
	if err := doJSON(client, request, &created, http.StatusOK, http.StatusCreated); err != nil {
		return wahooRoute{}, err
	}
	if created.ID <= 0 {
		return wahooRoute{}, errors.New("wahoo did not return a route id")
	}

	return created, nil
}

func getRoute(ctx context.Context, client *http.Client, baseURL *url.URL, accessToken string, routeID int64) (wahooRoute, error) {
	//nolint:gosec // The sandbox-only base URL is parsed as an HTTPS origin.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, routeURL(baseURL, fmt.Sprintf("/v1/routes/%d", routeID)), http.NoBody)
	if err != nil {
		return wahooRoute{}, fmt.Errorf("creating sandbox route request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)

	var fetched wahooRoute
	if err := doJSON(client, request, &fetched, http.StatusOK); err != nil {
		return wahooRoute{}, err
	}

	return fetched, nil
}

func deleteRoute(ctx context.Context, client *http.Client, baseURL *url.URL, accessToken string, routeID int64) error {
	//nolint:gosec // The sandbox-only base URL is parsed as an HTTPS origin.
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, routeURL(baseURL, fmt.Sprintf("/v1/routes/%d", routeID)), http.NoBody)
	if err != nil {
		return fmt.Errorf("creating sandbox route request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)

	return doJSON(client, request, nil, http.StatusOK, http.StatusNoContent)
}

func doJSON(client *http.Client, request *http.Request, output any, allowedStatuses ...int) (err error) {
	//nolint:gosec // Requests originate from a parsed sandbox HTTPS origin.
	response, err := client.Do(request)
	if err != nil {
		return errors.New("wahoo request failed")
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	if !slices.Contains(allowedStatuses, response.StatusCode) {
		return fmt.Errorf("wahoo returned HTTP %d", response.StatusCode)
	}
	if output == nil {
		return nil
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, wahooResponseLimit+1))
	if readErr != nil {
		return errors.New("unable to read wahoo response")
	}
	if len(body) > wahooResponseLimit {
		return errors.New("wahoo response exceeded size limit")
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("wahoo response was not valid json")
	}

	return nil
}

func parseSandboxBaseURL(value string) (*url.URL, error) {
	if value == "" {
		value = wahooSandboxBaseURL
	}
	baseURL, err := url.Parse(value)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.User != nil ||
		baseURL.RawQuery != "" || baseURL.Fragment != "" || (baseURL.Path != "" && baseURL.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return baseURL, nil
}

func routeURL(baseURL *url.URL, path string) string {
	endpoint := *baseURL
	endpoint.Path = path
	endpoint.RawPath = ""
	endpoint.RawQuery = ""

	return endpoint.String()
}

func sandboxExternalID() (string, error) {
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return "", fmt.Errorf("reading random bytes: %w", err)
	}

	return "domestique:fit-acceptance:" + hex.EncodeToString(random), nil
}

func sandboxStage(t *testing.T) route.Stage {
	t.Helper()
	stage, err := route.NewStage(
		route.ProviderVeloPlanner,
		999_999_999,
		1,
		"2026-08-17T00:00:00Z",
		"Domestique FIT acceptance",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		"sandbox-acceptance",
	)
	require.NoError(t, err)

	return stage
}
