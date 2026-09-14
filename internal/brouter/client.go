// Package brouter asks a BRouter routing engine to route an ordered set of
// waypoints and returns the snapped line it answers with.
package brouter

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
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const (
	defaultTimeout = 20 * time.Second
	// maximumBodyBytes bounds a routing response: a 300 km route is a few
	// hundred KB of GeoJSON, so this leaves ample headroom without being
	// unbounded.
	maximumBodyBytes = 8 << 20
)

// Failure categorises a routing failure without carrying anything of the
// engine's own response.
type Failure string

const (
	// FailureUnreachable is a transport error or a request that did not
	// complete within the adapter's timeout.
	FailureUnreachable Failure = "unreachable"
	// FailureRefused is a 4xx response: the request, or one of its points,
	// cannot be routed.
	FailureRefused Failure = "refused"
	// FailureEngine is a 5xx response from the engine itself.
	FailureEngine Failure = "engine"
	// FailureResponse is a 2xx response this adapter could not parse into a
	// usable route.
	FailureResponse Failure = "response"
)

// Error reports a routing failure by category and HTTP status alone; it never
// carries the engine's response body.
type Error struct {
	Category Failure
	Status   int
}

func (e *Error) Error() string {
	if e.Status == 0 {
		return fmt.Sprintf("brouter: %s", e.Category)
	}

	return fmt.Sprintf("brouter: %s (HTTP %d)", e.Category, e.Status)
}

// Options configures a BRouter client.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Timeout   time.Duration
}

// Waypoint is one point an engine is asked to route through.
type Waypoint struct {
	Longitude float64
	Latitude  float64
}

// Client asks one BRouter instance to route waypoints. The host is fixed at
// construction: BaseURL is the configured engine, sidecar or public instance,
// never a per-request value.
type Client struct {
	client  *http.Client
	baseURL *url.URL
	timeout time.Duration
}

// New creates a BRouter client without contacting the engine.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("brouter: options are required")
	}
	parsed, err := parseOrigin(options.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("brouter: base url: %w", err)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("brouter: timeout must be positive")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		client: &http.Client{
			Transport: transport,
			// A redirect is not a routing answer this adapter parses; left
			// alone it falls into the non-200 classification below.
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
		baseURL: parsed,
		timeout: timeout,
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

// Route asks the engine for the snapped line over waypoints for profile,
// returning one point per vertex of the returned track with an elevation
// where the engine supplied one.
func (c *Client) Route(
	ctx context.Context, waypoints []Waypoint, profile string,
) (points []route.Point, err error) {
	if len(waypoints) < 2 {
		return nil, &Error{Category: FailureRefused}
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := *c.baseURL
	endpoint.Path = "/brouter"
	endpoint.RawQuery = url.Values{
		"lonlats":        {lonlats(waypoints)},
		"profile":        {profile},
		"alternativeidx": {"0"},
		"format":         {"geojson"},
	}.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("brouter: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.geo+json, application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return nil, &Error{Category: FailureUnreachable}
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	// Classified before the body is read: only a 200 is worth the cost and the
	// exposure of parsing; every other status is drained and closed unread.
	switch {
	case response.StatusCode >= http.StatusInternalServerError:
		return nil, &Error{Category: FailureEngine, Status: response.StatusCode}
	case response.StatusCode >= http.StatusBadRequest:
		return nil, &Error{Category: FailureRefused, Status: response.StatusCode}
	case response.StatusCode != http.StatusOK:
		return nil, &Error{Category: FailureResponse, Status: response.StatusCode}
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return nil, &Error{Category: FailureUnreachable, Status: response.StatusCode}
	}
	if len(body) > maximumBodyBytes {
		return nil, &Error{Category: FailureResponse, Status: response.StatusCode}
	}

	return parseGeometry(body, response.StatusCode)
}

// lonlats formats waypoints as BRouter's pipe-separated lon,lat list.
// url.Values.Encode percent-encodes the pipe to %7C, which is what the
// server accepts.
func lonlats(waypoints []Waypoint) string {
	parts := make([]string, len(waypoints))
	for index, waypoint := range waypoints {
		parts[index] = strconv.FormatFloat(waypoint.Longitude, 'f', -1, 64) + "," +
			strconv.FormatFloat(waypoint.Latitude, 'f', -1, 64)
	}

	return strings.Join(parts, "|")
}

// featureCollection is the subset of BRouter's GeoJSON answer this adapter
// reads: one feature's LineString geometry.
type featureCollection struct {
	Features []struct {
		Geometry struct {
			Type        string      `json:"type"`
			Coordinates [][]float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

func parseGeometry(body []byte, status int) ([]route.Point, error) {
	var decoded featureCollection
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, &Error{Category: FailureResponse, Status: status}
	}
	if len(decoded.Features) == 0 || decoded.Features[0].Geometry.Type != "LineString" {
		return nil, &Error{Category: FailureResponse, Status: status}
	}
	coordinates := decoded.Features[0].Geometry.Coordinates
	if len(coordinates) < 2 {
		return nil, &Error{Category: FailureResponse, Status: status}
	}

	points := make([]route.Point, len(coordinates))
	for index, coordinate := range coordinates {
		if len(coordinate) < 2 {
			return nil, &Error{Category: FailureResponse, Status: status}
		}
		longitude, latitude := coordinate[0], coordinate[1]
		if !validCoordinate(longitude, -180, 180) || !validCoordinate(latitude, -90, 90) {
			return nil, &Error{Category: FailureResponse, Status: status}
		}
		point := route.Point{Longitude: longitude, Latitude: latitude}
		if len(coordinate) >= 3 {
			elevation := coordinate[2]
			if math.IsNaN(elevation) || math.IsInf(elevation, 0) {
				return nil, &Error{Category: FailureResponse, Status: status}
			}
			point.Elevation = &elevation
		}
		points[index] = point
	}

	return points, nil
}

// validCoordinate reports whether value is a finite number within [min, max].
func validCoordinate(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}
