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

// Configuration is one coherent set of values used for a notification send.
type Configuration struct {
	BaseURL          string
	ApplicationToken []byte
	UserKey          []byte
}

// Options configures a Pushover client.
type Options struct {
	Transport http.RoundTripper
	// Configuration is read once per send because an operator edits it at runtime.
	Configuration func() Configuration
	Timeout       time.Duration
}

// Client sends a bounded Pushover notification request.
type Client struct {
	client        *http.Client
	configuration func() Configuration
}

// New creates a Pushover client without contacting the upstream service.
func New(options *Options) (*Client, error) {
	if options == nil || options.Configuration == nil {
		return nil, errors.New("pushover: options and configuration are required")
	}
	// Checked once, against what it says now. A later edit is refused where it
	// is written, which is the only place that can still report the refusal.
	if _, err := parseOrigin(configurationBaseURL(options.Configuration().BaseURL)); err != nil {
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
		configuration: options.Configuration,
	}, nil
}

// Send delivers an already-safe title and message. It intentionally does not
// format synchronization data or inspect provider response text.
func (c *Client) Send(ctx context.Context, title, message string) (err error) {
	if strings.TrimSpace(title) == "" || strings.TrimSpace(message) == "" {
		return errors.New("pushover: title and message are required")
	}
	configuration := c.configuration()
	if len(configuration.ApplicationToken) == 0 || len(configuration.UserKey) == 0 {
		return errors.New("pushover: credentials are not set")
	}
	values := url.Values{
		"token":   {string(configuration.ApplicationToken)},
		"user":    {string(configuration.UserKey)},
		"title":   {title},
		"message": {message},
	}
	parsedBaseURL, err := parseOrigin(configurationBaseURL(configuration.BaseURL))
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

func configurationBaseURL(baseURL string) string {
	if baseURL == "" {
		return defaultBaseURL
	}

	return baseURL
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}
