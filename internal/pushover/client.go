// Package pushover sends already-safe Domestique notifications.
package pushover

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
)

const (
	defaultBaseURL   = "https://api.pushover.net"
	defaultTimeout   = 15 * time.Second
	maximumBodyBytes = 1 << 20
)

// Options configures a Pushover client with resolved static credentials.
type Options struct {
	Transport http.RoundTripper
	// BaseURL is read again before each send rather than resolved once, because
	// an operator edits it while the service runs.
	BaseURL          func() string
	ApplicationToken []byte
	UserKey          []byte
	Timeout          time.Duration
}

// Client sends a bounded Pushover notification request.
type Client struct {
	client           *http.Client
	baseURL          func() string
	applicationToken []byte
	userKey          []byte
}

// New creates a Pushover client without contacting the upstream service.
func New(options *Options) (*Client, error) {
	if options == nil || len(options.ApplicationToken) == 0 || len(options.UserKey) == 0 {
		return nil, errors.New("pushover: options and credentials are required")
	}
	baseURL := options.BaseURL
	if baseURL == nil {
		baseURL = func() string { return defaultBaseURL }
	}
	// Checked once, against what it says now. A later edit is refused where it
	// is written, which is the only place that can still report the refusal.
	if _, err := parseOrigin(baseURL()); err != nil {
		return nil, fmt.Errorf("pushover: base url: %w", err)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("pushover: timeout must be positive")
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
		baseURL:          baseURL,
		applicationToken: append([]byte(nil), options.ApplicationToken...),
		userKey:          append([]byte(nil), options.UserKey...),
	}, nil
}

// Send delivers an already-safe title and message. It intentionally does not
// format synchronization data or inspect provider response text.
func (c *Client) Send(ctx context.Context, title, message string) (err error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(message) == "" {
		return errors.New("pushover: title and message are required")
	}
	values := url.Values{
		"token":   {string(c.applicationToken)},
		"user":    {string(c.userKey)},
		"title":   {title},
		"message": {message},
	}
	parsedBaseURL, err := parseOrigin(c.baseURL())
	if err != nil {
		return fmt.Errorf("pushover: base url: %w", err)
	}
	endpoint := *parsedBaseURL
	endpoint.Path = "/1/messages.json"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("pushover: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := c.client.Do(request)
	if err != nil {
		return errors.New("pushover: request failed")
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("pushover: request returned HTTP %d", response.StatusCode)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return errors.New("pushover: response could not be read")
	}
	if len(body) > maximumBodyBytes {
		return errors.New("pushover: response exceeded size limit")
	}
	var payload struct {
		Status int `json:"status"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Status != 1 {
		return errors.New("pushover: notification was not accepted")
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
