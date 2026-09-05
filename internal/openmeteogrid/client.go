// Package openmeteogrid relays bytes from Open-Meteo's public spatial data
// files, one model's worth, to whoever asks. It decodes nothing: the reader on
// the other end already knows the ICON-D2 file format and its own chunking, so
// this owns exactly two upstream shapes and nothing about what is inside them.
package openmeteogrid

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

const (
	defaultBaseURL = "https://openmeteo.s3.amazonaws.com"
	defaultTimeout = 15 * time.Second
	// model is the one this service's browser bundle ever asks for. Fixed here
	// rather than taken from a caller, so the object key this package builds
	// can never be pointed at another path in the bucket.
	model = "dwd_icon_d2"

	// validStampFormat has no zone suffix: the bucket's own filenames drop it,
	// keeping only the two digits of the minute (always "00" in practice).
	validStampFormat = "2006-01-02T1504"
)

// Options configures an openmeteogrid client. There is no API key: the bucket
// is public.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Timeout   time.Duration
}

// Client relays Open-Meteo's spatial data. The host is hardcoded: BaseURL
// exists to be overridden by a test, not by an operator, the same as
// internal/openmeteo's.
type Client struct {
	client  *http.Client
	baseURL *url.URL
}

// New creates an openmeteogrid client without contacting the upstream bucket.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("openmeteogrid: options are required")
	}
	baseURL := options.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsedBaseURL, err := parseOrigin(baseURL)
	if err != nil {
		return nil, fmt.Errorf("openmeteogrid: base url: %w", err)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("openmeteogrid: timeout must be positive")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		client:  &http.Client{Timeout: timeout, Transport: transport},
		baseURL: parsedBaseURL,
	}, nil
}

// Latest fetches the model's own capture manifest: its reference time and the
// valid times it currently publishes. The caller decodes it; this package
// only moves the bytes.
func (c *Client) Latest(ctx context.Context) (*http.Response, error) {
	endpoint := *c.baseURL
	endpoint.Path = "/data_spatial/" + model + "/latest.json"

	return c.do(ctx, http.MethodGet, endpoint.String(), "")
}

// Object fetches one .om file's bytes, or answers a HEAD, for the run named by
// referenceTime and the hour named by validTime. rangeHeader is forwarded
// verbatim when non-empty — the caller's own reader decides which bytes of
// the file it needs and asks for them by byte range, this package has no say
// in that. The caller closes the returned response's body, the same contract
// http.Client.Do itself has.
func (c *Client) Object(
	ctx context.Context, referenceTime, validTime time.Time, method, rangeHeader string,
) (*http.Response, error) {
	if method != http.MethodGet && method != http.MethodHead {
		return nil, errors.New("openmeteogrid: method must be GET or HEAD")
	}
	utc := referenceTime.UTC()
	// The run's own hour, forced to :00 the way the bucket's own directories
	// are — mirrors openMeteoGrid.ts's omUrl exactly rather than trusting
	// whatever minute a caller's Date happens to carry.
	dir := fmt.Sprintf(
		"%04d/%02d/%02d/%02d00Z", utc.Year(), utc.Month(), utc.Day(), utc.Hour(),
	)
	endpoint := *c.baseURL
	endpoint.Path = "/data_spatial/" + model + "/" + dir + "/" + validTime.UTC().Format(validStampFormat) + ".om"

	return c.do(ctx, method, endpoint.String(), rangeHeader)
}

func (c *Client) do(ctx context.Context, method, endpoint, rangeHeader string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, method, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("openmeteogrid: creating request: %w", err)
	}
	if rangeHeader != "" {
		request.Header.Set("Range", rangeHeader)
	}
	// net/http adds Accept-Encoding: gzip and transparently decompresses
	// otherwise, desyncing the relayed ETag/Content-Length from the body this
	// package promises to pass through unchanged.
	request.Header.Set("Accept-Encoding", "identity")

	response, err := c.client.Do(request)
	if err != nil {
		return nil, errors.New("openmeteogrid: request failed")
	}

	return response, nil
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}
