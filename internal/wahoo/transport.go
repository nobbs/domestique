package wahoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

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

	client := &Client{
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
		apiBaseURL:   apiBaseURL,
		oauthBaseURL: oauthBaseURL,
		now:          time.Now,
		wait:         waitFor,
	}

	// AuthStyle is stated rather than detected: Wahoo takes the credentials as
	// form parameters, and autodetection probes, which spends a request out of a
	// daily quota this client exists to husband.
	client.oauth = &oauth2.Config{
		ClientID:     options.ClientID,
		ClientSecret: string(options.ClientSecret),
		RedirectURL:  options.RedirectURL,
		Scopes:       []string{"routes_read", "routes_write", "user_read", "workouts_read", "offline_data"},
		Endpoint: oauth2.Endpoint{
			AuthURL:   client.endpoint(oauthBaseURL, "/oauth/authorize").String(),
			TokenURL:  client.endpoint(oauthBaseURL, "/oauth/token").String(),
			AuthStyle: oauth2.AuthStyleInParams,
		},
	}
	// x/oauth2 drives its own requests, so it gets a client whose transport puts
	// them through the same throttle as every other call.
	client.oauthClient = &http.Client{
		Timeout:   timeout,
		Transport: &oauthTransport{client: client, base: transport},
	}

	return client, nil
}

// oauthTransport gates the token endpoint on the same mutex and quota as
// doJSON. Nothing nests: doJSON sends through the plain transport, only
// x/oauth2 sends through this one.
type oauthTransport struct {
	client *Client
	base   http.RoundTripper
}

func (t *oauthTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	client := t.client

	client.mutex.Lock()
	defer client.mutex.Unlock()

	if waitFor := client.notBefore.Sub(client.now()); waitFor > 0 {
		if waitFor > waitBudget(request.Context()) {
			return nil, ErrRateLimited
		}
		if waitErr := client.wait(request.Context(), waitFor); waitErr != nil {
			return nil, fmt.Errorf("wahoo: waiting for rate limit: %w", waitErr)
		}
	}

	response, err := t.base.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("wahoo: token request failed: %w", err)
	}
	client.observeRateLimit(response)

	return response, nil
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

// statusError is a response outside 2xx that no sentinel claims. It carries the
// status so a caller that knows its endpoint can tell a dead resource from a
// dead provider.
type statusError struct {
	status int
}

func (e *statusError) Error() string {
	return fmt.Sprintf("wahoo: request returned HTTP %d", e.status)
}

func (c *Client) doJSON(request *http.Request, output any) (err error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if waitFor := c.notBefore.Sub(c.now()); waitFor > 0 {
		if waitFor > waitBudget(request.Context()) {
			return ErrRateLimited
		}
		if waitErr := c.wait(request.Context(), waitFor); waitErr != nil {
			return fmt.Errorf("wahoo: waiting for rate limit: %w", waitErr)
		}
	}

	response, err := c.client.Do(request)
	if err != nil {
		// Keep the cause — a dial timeout reads differently from a TLS failure —
		// but drop the *url.Error wrapper, whose message carries the request URL.
		// Unwrapping also keeps errors.Is against a cancelled context working.
		var urlErr *url.Error
		if errors.As(err, &urlErr) { //nolint:modernize // errors.As is unambiguous to every tool reviewing this code.
			err = urlErr.Err
		}

		return fmt.Errorf("wahoo: request failed: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	c.observeRateLimit(response)
	// A rejected data request says nothing about the refresh token, which the
	// token endpoint alone judges (classifyTokenError); a 401 here is upstream.
	if response.StatusCode == http.StatusTooManyRequests {
		return ErrRateLimited
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return &statusError{status: response.StatusCode}
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

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
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
