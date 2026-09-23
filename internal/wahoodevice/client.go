// Package wahoodevice speaks the undocumented Wahoo API an ELEMNT device and
// its companion app sign in to, used only to set a route's provider_id, which
// the public Cloud API cannot write.
package wahoodevice

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the host the ELEMNT app reaches for sign-in and routes.
	DefaultBaseURL = "https://www.wahooligan.com"

	// fitnessAppID is the ELEMNT app's own id; the API answers a device session
	// only under it.
	fitnessAppID = "1"
	// userAgent mirrors what a BOLT v2 on firmware 1.77 sends.
	userAgent = "Dalvik/2.1.0 (Linux; U; Android 9; ELEMNT-BOLT2 Build/07172024) ELEMNT-BOLT/1.77.10.1"

	defaultTimeout   = 30 * time.Second
	maximumBodyBytes = 8 << 20
	maximumRoutes    = 5000
)

// ErrUnauthorized reports credentials the sign-in refused, or a session token
// a later call no longer accepts.
var ErrUnauthorized = errors.New("wahoodevice: authorization was rejected")

// Options configures a device API client.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Timeout   time.Duration
}

// Route is the part of a device-API route this service reads.
type Route struct {
	ExternalID string
	ProviderID string
	ID         int64
}

// Client is a Wahoo device API client.
type Client struct {
	httpClient *http.Client
	baseURL    *url.URL
}

// New creates a device API client without contacting Wahoo.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("wahoodevice: options are required")
	}
	baseURL, err := parseOrigin(cmp.Or(options.BaseURL, DefaultBaseURL))
	if err != nil {
		return nil, fmt.Errorf("wahoodevice: base url: %w", err)
	}
	timeout := cmp.Or(options.Timeout, defaultTimeout)
	if timeout < 0 {
		return nil, errors.New("wahoodevice: timeout must not be negative")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
			// A redirect would carry the password or WF-USER-TOKEN, which Go does
			// not strip, to whatever host it names.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		baseURL: baseURL,
	}, nil
}

// IsUnauthorized reports whether err is a refused sign-in or a session token
// that is no longer accepted.
func (c *Client) IsUnauthorized(err error) bool { return errors.Is(err, ErrUnauthorized) }

// SignIn opens a device session with a rider's own Wahoo email and password.
// The API offers no way to end one, so the token stays valid until Wahoo
// drops it.
func (c *Client) SignIn(ctx context.Context, email, password []byte) (string, error) {
	if len(email) == 0 || len(password) == 0 {
		return "", errors.New("wahoodevice: email and password are required")
	}
	form := url.Values{"email": {string(email)}, "password": {string(password)}}
	request, err := c.newRequest(ctx, http.MethodPost, "/api/v1/sessions/", nil, strings.NewReader(form.Encode()), "")
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var response struct {
		Token string `json:"token"`
	}
	if err := c.do(request, &response); err != nil {
		return "", err
	}
	if response.Token == "" {
		return "", errors.New("wahoodevice: sign-in response carried no token")
	}

	return response.Token, nil
}

// Routes lists every route the signed-in account holds that is not deleted.
func (c *Client) Routes(ctx context.Context, token string) ([]Route, error) {
	if token == "" {
		return nil, errors.New("wahoodevice: session token is required")
	}
	request, err := c.newRequest(ctx, http.MethodGet, "/api/v1/routes", url.Values{"deleted": {"0"}}, http.NoBody, token)
	if err != nil {
		return nil, err
	}

	var response []struct {
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		ExternalID *string `json:"external_id"`
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		ProviderID json.RawMessage `json:"provider_id"`
		ID         int64           `json:"id"`
	}
	if err := c.do(request, &response); err != nil {
		return nil, err
	}
	if len(response) > maximumRoutes {
		return nil, errors.New("wahoodevice: route listing exceeded configured bounds")
	}

	routes := make([]Route, 0, len(response))
	for _, item := range response {
		if item.ID <= 0 {
			continue
		}
		provider, known := providerID(item.ProviderID)
		if !known {
			return nil, errors.New("wahoodevice: route listing carried an unreadable provider_id")
		}
		routes = append(routes, Route{
			ID:         item.ID,
			ExternalID: derefString(item.ExternalID),
			ProviderID: provider,
		})
	}

	return routes, nil
}

// SetProviderID writes a route's provider_id, the value an ELEMNT keys the
// route by on the device.
func (c *Client) SetProviderID(ctx context.Context, token string, routeID int64, value string) error {
	if token == "" || routeID <= 0 || value == "" {
		return errors.New("wahoodevice: session token, route id and value are required")
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("route[provider_id]", value); err != nil {
		return fmt.Errorf("wahoodevice: encoding the route form: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("wahoodevice: encoding the route form: %w", err)
	}
	request, err := c.newRequest(ctx, http.MethodPut, "/api/v1/routes/"+strconv.FormatInt(routeID, 10), nil, &body, token)
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())

	return c.do(request, nil)
}

func (c *Client) newRequest(
	ctx context.Context, method, path string, query url.Values, body io.Reader, token string,
) (*http.Request, error) {
	endpoint := *c.baseURL
	endpoint.Path = path
	endpoint.RawPath = ""
	endpoint.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return nil, fmt.Errorf("wahoodevice: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("WF-FITNESS-APP-ID", fitnessAppID)
	if token != "" {
		request.Header.Set("WF-USER-TOKEN", token)
	}

	return request, nil
}

// do sends request and, when output is not nil, decodes a JSON reply into it.
// Errors carry only the status, never a response body.
func (c *Client) do(request *http.Request, output any) (err error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}

		return fmt.Errorf("wahoodevice: request failed: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	switch status := response.StatusCode; {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
	// Only 401 speaks for the credentials or the session: a wrong password gets
	// one, a malformed sign-in 422, and a 403 may come from a filter in front.
	case status == http.StatusUnauthorized:
		return fmt.Errorf("%w: HTTP %d", ErrUnauthorized, status)
	default:
		return fmt.Errorf("wahoodevice: request returned HTTP %d", status)
	}
	if output == nil {
		return nil
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return errors.New("wahoodevice: response could not be read")
	}
	if len(body) > maximumBodyBytes {
		return errors.New("wahoodevice: response exceeded size limit")
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("wahoodevice: response was not valid json")
	}

	return nil
}

// providerID reads a provider_id Wahoo sends as a string, a number or null.
// Any other shape, an absent field included, is not known to be empty.
func providerID(raw json.RawMessage) (string, bool) {
	if string(raw) == "null" {
		return "", true
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, true
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return number.String(), true
	}

	return "", false
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}
