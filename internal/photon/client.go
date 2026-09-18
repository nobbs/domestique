// Package photon asks a Photon geocoder what a coordinate is called, returning
// one short label or nothing where the place has no name worth showing, and
// which places answer to a name. It holds the names it has been given so a
// coordinate asked for twice costs one request; searches are not held.
package photon

import (
	"context"
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
)

const (
	defaultTimeout = 10 * time.Second
	// defaultMinInterval paces requests to the public instance, whose terms are
	// fair use alone. A self-hosted instance can set it to zero.
	defaultMinInterval = 200 * time.Millisecond
	// maximumBodyBytes bounds an answer: at most a search's few features, each a
	// handful of string properties, so this is already generous.
	maximumBodyBytes = 1 << 20
	// cacheLimit bounds what one process remembers; a plan holds at most fifty
	// waypoints, so this is many plans' worth of short strings.
	cacheLimit = 4096
	// cachePrecision rounds a coordinate to about eleven metres, which is finer
	// than any two waypoints a rider would call the same place.
	cachePrecision = 4
)

// Failure categorises a lookup failure without carrying anything of the
// geocoder's own response.
type Failure string

const (
	// FailureUnreachable is a transport error or a request that did not
	// complete within the adapter's timeout.
	FailureUnreachable Failure = "unreachable"
	// FailureRefused is a 4xx response: the coordinate, as asked for, was not
	// accepted.
	FailureRefused Failure = "refused"
	// FailureGeocoder is a 5xx response from the geocoder itself.
	FailureGeocoder Failure = "geocoder"
	// FailureResponse is a 2xx response this adapter could not parse.
	FailureResponse Failure = "response"
)

// Error reports a lookup failure by category and HTTP status alone; it never
// carries the geocoder's response body.
type Error struct {
	Category Failure
	Status   int
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("photon: %s", e.Category)
	}

	return fmt.Sprintf("photon: %s (HTTP %d)", e.Category, e.Status)
}

// Options configures a Photon client.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Timeout   time.Duration
	// MinInterval is the least time between two upstream requests. Zero takes
	// the default; negative is refused.
	MinInterval time.Duration
}

// Client asks one Photon instance what coordinates are called. The host is
// fixed at construction, never a per-request value.
type Client struct {
	client  *http.Client
	baseURL *url.URL
	names   map[string]string
	// lastCall and upstream serialise calls to the geocoder, so a plan's
	// waypoints trickle rather than burst; remembered guards names.
	lastCall    time.Time
	timeout     time.Duration
	minInterval time.Duration
	upstream    sync.Mutex
	remembered  sync.RWMutex
}

// New creates a Photon client without contacting the geocoder.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("photon: options are required")
	}
	parsed, err := parseOrigin(options.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("photon: base url: %w", err)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("photon: timeout must be positive")
	}
	interval := options.MinInterval
	if interval == 0 {
		interval = defaultMinInterval
	}
	if interval < 0 {
		return nil, errors.New("photon: minimum interval must not be negative")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		client: &http.Client{
			Transport: transport,
			// A redirect is not an answer this adapter parses; left alone it
			// falls into the non-200 classification below.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		baseURL:     parsed,
		timeout:     timeout,
		minInterval: interval,
		names:       make(map[string]string),
	}, nil
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute http or https origin")
	}

	return parsed, nil
}

// Reverse names the place at a coordinate. An empty name is an answer, not a
// failure: open country has nothing to be called.
func (c *Client) Reverse(ctx context.Context, latitude, longitude float64) (string, error) {
	if math.IsNaN(latitude) || math.IsNaN(longitude) ||
		math.Abs(latitude) > 90 || math.Abs(longitude) > 180 {
		return "", &Error{Category: FailureRefused}
	}
	key := cacheKey(latitude, longitude)
	if cached, ok := c.recall(key); ok {
		return cached, nil
	}

	name, err := c.lookup(ctx, latitude, longitude)
	if err != nil {
		return "", err
	}
	c.remember(key, name)

	return name, nil
}

func (c *Client) recall(key string) (string, bool) {
	c.remembered.RLock()
	defer c.remembered.RUnlock()
	name, ok := c.names[key]

	return name, ok
}

// remember keeps an answer, emptying the map once it has grown past its bound
// rather than evicting by age: a refill costs one request per waypoint in view.
func (c *Client) remember(key, name string) {
	c.remembered.Lock()
	defer c.remembered.Unlock()
	if len(c.names) >= cacheLimit {
		clear(c.names)
	}
	c.names[key] = name
}

func (c *Client) lookup(ctx context.Context, latitude, longitude float64) (string, error) {
	body, status, err := c.get(ctx, "/reverse", url.Values{
		"lat":   {strconv.FormatFloat(latitude, 'f', -1, 64)},
		"lon":   {strconv.FormatFloat(longitude, 'f', -1, 64)},
		"limit": {"1"},
	})
	if err != nil {
		return "", err
	}

	return parseReverse(body, status)
}

// get asks the geocoder one paced, bounded question and answers the body of a
// 200; every other outcome is an *Error carrying nothing of the response.
func (c *Client) get(ctx context.Context, path string, query url.Values) (body []byte, status int, err error) {
	if waitErr := c.pace(ctx); waitErr != nil {
		return nil, 0, waitErr
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := *c.baseURL
	endpoint.Path = path
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, 0, fmt.Errorf("photon: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return nil, 0, &Error{Category: FailureUnreachable}
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	status = response.StatusCode
	// Classified before the body is read: only a 200 is worth parsing, every
	// other status is closed unread.
	switch {
	case status >= http.StatusInternalServerError:
		return nil, status, &Error{Category: FailureGeocoder, Status: status}
	case status >= http.StatusBadRequest:
		return nil, status, &Error{Category: FailureRefused, Status: status}
	case status != http.StatusOK:
		return nil, status, &Error{Category: FailureResponse, Status: status}
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return nil, status, &Error{Category: FailureUnreachable, Status: status}
	}
	if len(body) > maximumBodyBytes {
		return nil, status, &Error{Category: FailureResponse, Status: status}
	}

	return body, status, nil
}

// pace holds the caller until the minimum interval since the last request has
// passed, so several waypoints resolving at once do not arrive as a burst.
func (c *Client) pace(ctx context.Context) error {
	c.upstream.Lock()
	defer c.upstream.Unlock()

	wait := c.minInterval - time.Since(c.lastCall)
	if wait > 0 && !c.lastCall.IsZero() {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return &Error{Category: FailureUnreachable}
		case <-timer.C:
		}
	}
	c.lastCall = time.Now()

	return nil
}

func cacheKey(latitude, longitude float64) string {
	return strconv.FormatFloat(round(latitude), 'f', cachePrecision, 64) + "," +
		strconv.FormatFloat(round(longitude), 'f', cachePrecision, 64)
}

func round(value float64) float64 {
	scale := math.Pow10(cachePrecision)

	return math.Round(value*scale) / scale
}

// properties is the subset of a Photon feature's properties a label is built
// from; the answer carries many more.
type properties struct {
	Name        string `json:"name"`
	Street      string `json:"street"`
	HouseNumber string `json:"housenumber"`
	District    string `json:"district"`
	City        string `json:"city"`
	County      string `json:"county"`
	State       string `json:"state"`
}

type featureCollection struct {
	Features []struct {
		Properties properties `json:"properties"`
	} `json:"features"`
}

func parseReverse(body []byte, status int) (string, error) {
	var decoded featureCollection
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", &Error{Category: FailureResponse, Status: status}
	}
	if len(decoded.Features) == 0 {
		return "", nil
	}

	return label(&decoded.Features[0].Properties), nil
}

// label is the one short name for a place: what it is, within the enclosing
// place where that says more.
func label(from *properties) string {
	return join(what(from), firstOf(from.City, from.District, from.County, from.State))
}

// what names a place by the address where there is one, its own name where it
// has one, and the enclosing place otherwise.
func what(from *properties) string {
	switch {
	case from.Street != "" && from.HouseNumber != "":
		return from.Street + " " + from.HouseNumber
	case from.Name != "":
		return from.Name
	case from.Street != "":
		return from.Street
	default:
		return firstOf(from.City, from.District, from.County, from.State)
	}
}

// join names a place within the wider one, unless they are the same place.
func join(what, where string) string {
	if where == "" || strings.EqualFold(what, where) {
		return what
	}

	return what + ", " + where
}

func firstOf(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
