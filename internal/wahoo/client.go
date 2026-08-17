// Package wahoo speaks Wahoo's OAuth and route APIs.
package wahoo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const (
	defaultTimeout   = 30 * time.Second
	maximumBodyBytes = 1 << 20
	earthRadiusMetre = 6_371_000.0
)

var (
	// ErrUnauthorized reports a rejected Wahoo authorization or refresh token.
	ErrUnauthorized = errors.New("wahoo authorization was rejected")
	// ErrRateLimited reports a request rejected by Wahoo's advertised rate limit.
	ErrRateLimited = errors.New("wahoo rate limit was reached")
)

// Options configures a Wahoo API client with resolved OAuth credentials.
type Options struct {
	Transport    http.RoundTripper
	APIBaseURL   string
	OAuthBaseURL string
	ClientID     string
	RedirectURL  string
	ClientSecret []byte
	Timeout      time.Duration
}

// Client is a serial Wahoo API client with shared rate-limit handling.
type Client struct {
	notBefore    time.Time
	client       *http.Client
	apiBaseURL   *url.URL
	oauthBaseURL *url.URL
	now          func() time.Time
	wait         func(context.Context, time.Duration) error
	clientID     string
	redirectURL  string
	clientSecret []byte
	mutex        sync.Mutex
}

// New creates a Wahoo client without contacting the API.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("wahoo: options are required")
	}
	apiBaseURL, err := parseOrigin(options.APIBaseURL)
	if err != nil {
		return nil, fmt.Errorf("wahoo: api base url: %w", err)
	}
	oauthBaseURL, err := parseOrigin(options.OAuthBaseURL)
	if err != nil {
		return nil, fmt.Errorf("wahoo: oauth base url: %w", err)
	}
	if strings.TrimSpace(options.ClientID) == "" || strings.TrimSpace(options.RedirectURL) == "" || len(options.ClientSecret) == 0 {
		return nil, errors.New("wahoo: client id, client secret, and redirect url are required")
	}
	if _, err := parseCallbackURL(options.RedirectURL); err != nil {
		return nil, errors.New("wahoo: redirect url must be an https oauth callback url")
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("wahoo: timeout must be positive")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		apiBaseURL:   apiBaseURL,
		oauthBaseURL: oauthBaseURL,
		clientID:     options.ClientID,
		redirectURL:  options.RedirectURL,
		clientSecret: append([]byte(nil), options.ClientSecret...),
		now:          time.Now,
		wait:         waitFor,
	}, nil
}

// AuthorizationURL returns a confidential-client Wahoo authorization URL for a
// one-time opaque state value.
func (c *Client) AuthorizationURL(state string) (string, error) {
	if state == "" {
		return "", errors.New("wahoo: oauth state is required")
	}

	endpoint := c.endpoint(c.oauthBaseURL, "/oauth/authorize")
	query := endpoint.Query()
	query.Set("client_id", c.clientID)
	query.Set("redirect_uri", c.redirectURL)
	query.Set("response_type", "code")
	query.Set("scope", "routes_read routes_write user_read")
	query.Set("state", state)
	endpoint.RawQuery = query.Encode()

	return endpoint.String(), nil
}

// ExchangeAuthorizationCode trades a Wahoo authorization code for fresh tokens.
func (c *Client) ExchangeAuthorizationCode(ctx context.Context, code string) (accessToken, refreshToken string, err error) {
	if code == "" {
		return "", "", errors.New("wahoo: authorization code is required")
	}

	return c.requestToken(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {string(c.clientSecret)},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {c.redirectURL},
	})
}

// RefreshAccessToken obtains a replacement access and refresh token immediately
// before a Wahoo API request.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	if refreshToken == "" {
		return "", "", errors.New("wahoo: refresh token is required")
	}

	return c.requestToken(ctx, url.Values{
		"client_id":     {c.clientID},
		"client_secret": {string(c.clientSecret)},
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
}

// AuthenticatedUser returns the stable Wahoo user identity for an access token.
func (c *Client) AuthenticatedUser(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", errors.New("wahoo: access token is required")
	}

	request, err := c.newRequest(ctx, http.MethodGet, c.endpoint(c.apiBaseURL, "/v1/user"), http.NoBody, accessToken)
	if err != nil {
		return "", err
	}
	var response struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(request, &response); err != nil {
		return "", err
	}
	if response.ID <= 0 {
		return "", errors.New("wahoo: user response did not contain an id")
	}

	return strconv.FormatInt(response.ID, 10), nil
}

// CreateRoute uploads a FIT course as a Wahoo route owned by Domestique.
func (c *Client) CreateRoute(ctx context.Context, accessToken string, stage *route.Stage, fitData []byte) (routeID int64, err error) {
	return c.writeRoute(ctx, http.MethodPost, 0, accessToken, stage, fitData)
}

// UpdateRoute replaces the FIT course and mutable metadata of an owned route.
func (c *Client) UpdateRoute(ctx context.Context, routeID int64, accessToken string, stage *route.Stage, fitData []byte) (updatedRouteID int64, err error) {
	if routeID <= 0 {
		return 0, errors.New("wahoo: route id must be positive")
	}

	return c.writeRoute(ctx, http.MethodPut, routeID, accessToken, stage, fitData)
}

// RouteByExternalID looks up an owned route by its deterministic external ID.
// A missing route returns found=false without treating the lookup as an error.
func (c *Client) RouteByExternalID(ctx context.Context, accessToken, externalID string) (routeID int64, found bool, err error) {
	if accessToken == "" || externalID == "" {
		return 0, false, errors.New("wahoo: access token and external id are required")
	}

	endpoint := c.endpoint(c.apiBaseURL, "/v1/routes")
	query := endpoint.Query()
	query.Set("external_id", externalID)
	endpoint.RawQuery = query.Encode()
	request, err := c.newRequest(ctx, http.MethodGet, endpoint, http.NoBody, accessToken)
	if err != nil {
		return 0, false, err
	}
	var response []routeResponse
	if err := c.doJSON(request, &response); err != nil {
		return 0, false, err
	}
	if len(response) == 0 {
		return 0, false, nil
	}
	if len(response) > 1 || response[0].ID <= 0 || response[0].ExternalID != externalID {
		return 0, false, errors.New("wahoo: route lookup returned an invalid result")
	}

	return response[0].ID, true, nil
}

// IsUnauthorized reports whether err is a permanent Wahoo authorization
// rejection. Consumers use it to request an interactive reauthorization.
func (c *Client) IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// DeleteRoute removes one route previously identified as Domestique-owned.
func (c *Client) DeleteRoute(ctx context.Context, routeID int64, accessToken string) error {
	if routeID <= 0 || accessToken == "" {
		return errors.New("wahoo: route id and access token are required")
	}
	request, err := c.newRequest(ctx, http.MethodDelete, c.endpoint(c.apiBaseURL, fmt.Sprintf("/v1/routes/%d", routeID)), http.NoBody, accessToken)
	if err != nil {
		return err
	}

	return c.doJSON(request, nil)
}

type tokenResponse struct {
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	AccessToken string `json:"access_token"`
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	RefreshToken string `json:"refresh_token"`
}

type routeResponse struct {
	//nolint:tagliatelle // Wahoo's API uses snake_case.
	ExternalID string `json:"external_id"`
	ID         int64  `json:"id"`
}

func (c *Client) requestToken(ctx context.Context, values url.Values) (accessToken, refreshToken string, err error) {
	request, err := c.newRequest(
		ctx,
		http.MethodPost,
		c.endpoint(c.oauthBaseURL, "/oauth/token"),
		strings.NewReader(values.Encode()),
		"",
	)
	if err != nil {
		return "", "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var response tokenResponse
	if err := c.doJSON(request, &response); err != nil {
		return "", "", err
	}
	if response.AccessToken == "" || response.RefreshToken == "" {
		return "", "", errors.New("wahoo: token response was incomplete")
	}

	return response.AccessToken, response.RefreshToken, nil
}

func (c *Client) writeRoute(
	ctx context.Context,
	method string,
	existingRouteID int64,
	accessToken string,
	stage *route.Stage,
	fitData []byte,
) (routeID int64, err error) {
	if accessToken == "" || stage == nil || len(fitData) == 0 {
		return 0, errors.New("wahoo: access token, route stage, and fit data are required")
	}

	geometry := stage.Geometry()
	metrics := calculateMetrics(geometry)
	values := url.Values{
		"route[file]":                   {"data:application/vnd.fit;base64," + base64.StdEncoding.EncodeToString(fitData)},
		"route[filename]":               {"domestique.fit"},
		"route[provider_updated_at]":    {stage.Revision()},
		"route[name]":                   {stage.Title()},
		"route[workout_type_family_id]": {"0"},
		"route[start_lat]":              {formatFloat(geometry[0].Latitude)},
		"route[start_lng]":              {formatFloat(geometry[0].Longitude)},
		"route[distance]":               {formatFloat(metrics.distance)},
		"route[ascent]":                 {formatFloat(metrics.ascent)},
		"route[descent]":                {formatFloat(metrics.descent)},
	}
	if method == http.MethodPost {
		values.Set("route[external_id]", stage.Key().ExternalID())
	}

	path := "/v1/routes"
	if existingRouteID > 0 {
		path = fmt.Sprintf("/v1/routes/%d", existingRouteID)
	}
	request, err := c.newRequest(ctx, method, c.endpoint(c.apiBaseURL, path), strings.NewReader(values.Encode()), accessToken)
	if err != nil {
		return 0, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var response routeResponse
	if err := c.doJSON(request, &response); err != nil {
		return 0, err
	}
	if response.ID <= 0 || (method == http.MethodPost && response.ExternalID != stage.Key().ExternalID()) {
		return 0, errors.New("wahoo: route response did not contain the expected route")
	}

	return response.ID, nil
}

func (c *Client) newRequest(ctx context.Context, method string, endpoint *url.URL, body io.Reader, accessToken string) (*http.Request, error) {

	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("wahoo: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	if accessToken != "" {
		request.Header.Set("Authorization", "Bearer "+accessToken)
	}

	return request, nil
}

func (c *Client) doJSON(request *http.Request, output any) (err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if waitFor := c.notBefore.Sub(c.now()); waitFor > 0 {
		if waitErr := c.wait(request.Context(), waitFor); waitErr != nil {
			return fmt.Errorf("wahoo: waiting for rate limit: %w", waitErr)
		}
	}

	response, err := c.client.Do(request)
	if err != nil {
		return errors.New("wahoo: request failed")
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	c.observeRateLimit(response)
	if response.StatusCode == http.StatusUnauthorized ||
		(response.StatusCode == http.StatusBadRequest && request.URL.Path == "/oauth/token") {
		return ErrUnauthorized
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("wahoo: request returned HTTP %d", response.StatusCode)
	}
	if output == nil {
		return nil
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return errors.New("wahoo: response could not be read")
	}
	if len(body) > maximumBodyBytes {
		return errors.New("wahoo: response exceeded size limit")
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("wahoo: response was not valid json")
	}

	return nil
}

func (c *Client) observeRateLimit(response *http.Response) {
	remaining, reset, ok := rateLimit(response.Header)
	if !ok || remaining > 0 || reset <= 0 {
		return
	}
	c.notBefore = c.now().Add(reset)
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}

func parseCallbackURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "/oauth/wahoo/callback" {
		return nil, errors.New("must be an absolute https oauth callback url")
	}

	return parsed, nil
}

func (c *Client) endpoint(baseURL *url.URL, path string) *url.URL {
	endpoint := *baseURL
	endpoint.Path = path
	endpoint.RawPath = ""
	endpoint.RawQuery = ""

	return &endpoint
}

func rateLimit(header http.Header) (int, time.Duration, bool) {
	remaining, ok := lowestRateLimit(header.Get("X-RateLimit-Remaining"))
	if !ok {
		return 0, 0, false
	}
	seconds, err := strconv.ParseInt(strings.TrimSpace(header.Get("X-RateLimit-Reset")), 10, 64)
	if err != nil || seconds < 0 {
		return 0, 0, false
	}

	return remaining, time.Duration(seconds) * time.Second, true
}

func lowestRateLimit(value string) (int, bool) {
	lowest := math.MaxInt
	for item := range strings.SplitSeq(value, ",") {
		remaining, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || remaining < 0 {
			return 0, false
		}
		lowest = min(lowest, remaining)
	}
	if lowest == math.MaxInt {
		return 0, false
	}

	return lowest, true
}

func calculateMetrics(geometry []route.Point) routeMetrics {
	var metrics routeMetrics
	for index := 1; index < len(geometry); index++ {
		metrics.distance += haversine(geometry[index-1], geometry[index])
		if geometry[index-1].Elevation != nil && geometry[index].Elevation != nil {
			delta := *geometry[index].Elevation - *geometry[index-1].Elevation
			if delta > 0 {
				metrics.ascent += delta
			} else {
				metrics.descent -= delta
			}
		}
	}

	return metrics
}

type routeMetrics struct {
	distance float64
	ascent   float64
	descent  float64
}

func haversine(left, right route.Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return earthRadiusMetre * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func waitFor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wahoo: rate limit wait cancelled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
