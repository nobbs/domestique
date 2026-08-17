package surface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// DefaultEndpoint is the public Overpass instance. It is the default because it
// needs no account and no local service, which is the whole reason this feature
// is affordable at all. It is also a volunteer-run server under a fair use
// policy, so every query here is shaped to ask for as little as it can.
const DefaultEndpoint = "https://overpass-api.de/api/interpreter"

const (
	// defaultTimeout is generous because an Overpass query is not a page load.
	// The server queues requests behind whatever else it is serving, and a query
	// that would have answered in twenty seconds on an idle instance can take
	// minutes on a busy one.
	defaultTimeout = 180 * time.Second

	// queryTimeoutSeconds is the budget the server itself is told to apply, and is
	// deliberately below defaultTimeout. Overpass abandons a query that exceeds it
	// and says so, which is a far better outcome than the client hanging up on a
	// query the server then finishes and discards.
	queryTimeoutSeconds = 150

	// maximumBodyBytes bounds the response. Dense urban geometry runs to a few
	// hundred kilobytes for every ten kilometres ridden, so this leaves room for
	// the longest stage anyone would sensibly plan while still refusing to read an
	// unbounded body into memory.
	maximumBodyBytes = 32 << 20

	// userAgent identifies this client to the endpoint. The Overpass fair use
	// policy asks for it, and an operator who can see what is generating traffic
	// can ask it to stop instead of blocking the whole address.
	userAgent = "domestique/1.0 (+https://github.com/nobbs/domestique)"

	// queryToleranceMetres is how far the queried corridor may drift from the
	// route where it is simplified.
	queryToleranceMetres = 10.0

	// queryRadiusMetres is how far either side of the corridor to ask about, and
	// is the snap radius widened by the drift the simplification is allowed. That
	// sum is what makes the two settings safe together: wherever the corridor has
	// drifted its full tolerance away from the route, the query still reaches
	// everything within snapRadiusMetres of the route itself, so the match can
	// never want a way the query declined to return.
	queryRadiusMetres = snapRadiusMetres + queryToleranceMetres

	// maximumQueryPoints caps the vertices in one around filter. Overpass slows
	// sharply as that count grows — the cost is the polyline against the index,
	// not the ways returned — so a long stage is asked for in several queries
	// rather than one the endpoint would labour over.
	maximumQueryPoints = 250

	// rateLimitPause is how long to wait before retrying a refused query. The
	// Overpass documentation asks for thirty seconds when a slot is unavailable.
	rateLimitPause = 30 * time.Second
)

// ErrRateLimited reports that the endpoint refused a query for want of capacity
// and refused it again after a pause. It is separate from an ordinary failure
// because the remedy is different: there is nothing wrong with the request, and
// the caller should stop asking for now rather than work through the rest of its
// queue against a server that is already turning work away.
var ErrRateLimited = errors.New("surface: overpass rate limited")

// errBusy marks the statuses Overpass uses to say it has no capacity right now,
// so post can tell a refusal apart from a real failure.
var errBusy = errors.New("endpoint busy")

// Options configures an Overpass client.
type Options struct {
	// Transport is the round tripper to use, for tests and for callers that
	// share a connection pool. The default transport is used when it is nil.
	Transport http.RoundTripper
	// Endpoint is the Overpass API interpreter URL. DefaultEndpoint is used when
	// it is empty.
	Endpoint string
	// Timeout bounds one query, including the pause the server spends queueing
	// it. defaultTimeout is used when it is zero.
	Timeout time.Duration
}

// Overpass reads the OpenStreetMap ways along a route from an Overpass API
// endpoint.
//
// It lives in this package rather than in an adapter of its own because it has
// no use outside classification: it exists to turn stage geometry into
// classified candidate ways, and splitting it out would only add a second set
// of types to convert between.
type Overpass struct {
	transport http.RoundTripper
	endpoint  *url.URL
	wait      func(context.Context, time.Duration) error
	timeout   time.Duration
}

// NewOverpass creates a client without contacting the endpoint.
func NewOverpass(options *Options) (*Overpass, error) {
	if options == nil {
		options = &Options{}
	}

	address := options.Endpoint
	if address == "" {
		address = DefaultEndpoint
	}
	endpoint, err := url.Parse(address)
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Host == "" {
		return nil, errors.New("surface: overpass endpoint must be an absolute http or https url")
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("surface: overpass timeout must be positive")
	}

	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Overpass{endpoint: endpoint, timeout: timeout, transport: transport, wait: waitFor}, nil
}

// waitFor pauses, or gives up early if the caller's context is done.
func waitFor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("surface: overpass retry wait cancelled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

// Ways returns the classified ways near the given stage geometry, in the order
// the endpoint reported them.
//
// A long stage is asked for in several queries, and a way lying under more than
// one of them is returned once. Every way the endpoint reports is kept, whether
// or not its surface is recorded: an unsurveyed road left out of the candidates
// would hand its stretch of the route to whatever else is nearby, which reads
// as a surveyed surface when it is nothing of the kind.
func (o *Overpass) Ways(ctx context.Context, points []route.Point) ([]Way, error) {
	if len(points) < 2 {
		return nil, nil
	}

	ways := make([]Way, 0)
	seen := make(map[int64]bool)
	for _, chunk := range chunkPoints(decimate(points, queryToleranceMetres), maximumQueryPoints) {
		body, err := o.post(ctx, buildQuery(chunk))
		if err != nil {
			return nil, err
		}
		decoded, err := decodeWays(body)
		if err != nil {
			return nil, err
		}
		for _, way := range decoded {
			if seen[way.ID] {
				continue
			}
			seen[way.ID] = true
			ways = append(ways, way)
		}
	}

	return ways, nil
}

// post sends one query, pausing once and retrying if the endpoint says it is
// busy. A single retry rides out a moment's contention; a second refusal means
// the endpoint is saturated, and saying so as ErrRateLimited lets the caller
// give up on this run rather than queue behind it.
func (o *Overpass) post(ctx context.Context, query string) ([]byte, error) {
	client := &http.Client{Transport: o.transport, Timeout: o.timeout}

	body, wait, err := o.attempt(ctx, client, query)
	if !errors.Is(err, errBusy) {
		return body, err
	}

	if waitErr := o.wait(ctx, wait); waitErr != nil {
		return nil, waitErr
	}

	body, _, err = o.attempt(ctx, client, query)
	if errors.Is(err, errBusy) {
		return nil, ErrRateLimited
	}

	return body, err
}

// attempt performs one request. A busy endpoint is reported as errBusy along
// with how long it asked to be left alone.
func (o *Overpass) attempt(
	ctx context.Context,
	client *http.Client,
	query string,
) (body []byte, wait time.Duration, err error) {
	form := url.Values{"data": []string{query}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, o.endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("surface: building overpass request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", userAgent)

	response, err := client.Do(request)
	if err != nil {
		return nil, 0, fmt.Errorf("surface: sending overpass query: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	body, err = io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if err != nil {
		return nil, 0, fmt.Errorf("surface: reading overpass response: %w", err)
	}
	if len(body) > maximumBodyBytes {
		return nil, 0, errors.New("surface: overpass response exceeded size limit")
	}

	switch response.StatusCode {
	case http.StatusOK:
		return body, 0, nil
	case http.StatusTooManyRequests, http.StatusGatewayTimeout:
		return nil, retryAfter(response.Header.Get("Retry-After")), errBusy
	}

	return nil, 0, fmt.Errorf("surface: overpass returned HTTP %d", response.StatusCode)
}

// retryAfter reads the header of that name, falling back to rateLimitPause. A
// server that names its own interval knows better than this client does, but a
// value far in the future is not worth holding a sync open for.
func retryAfter(header string) time.Duration {
	seconds, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || seconds <= 0 {
		return rateLimitPause
	}
	wait := time.Duration(seconds) * time.Second
	if wait > 2*rateLimitPause {
		return 2 * rateLimitPause
	}

	return wait
}

// buildQuery writes the Overpass QL for the ways along one run of geometry.
//
// The around filter buffers the polyline it is given rather than each vertex, so
// this asks for everything within queryRadiusMetres of the route as ridden. The
// highway filter keeps the answer to things that are actually roads and paths,
// and the exclusions drop the tagged-as-highway features nothing is ridden on:
// a route beside a bus platform or a road under construction should not be able
// to take its surface from one.
func buildQuery(points []route.Point) string {
	var builder strings.Builder
	builder.WriteString("[out:json][timeout:")
	builder.WriteString(strconv.Itoa(queryTimeoutSeconds))
	builder.WriteString("];way(around:")
	builder.WriteString(strconv.FormatFloat(queryRadiusMetres, 'f', -1, 64))
	for _, point := range points {
		builder.WriteByte(',')
		builder.WriteString(strconv.FormatFloat(point.Latitude, 'f', 5, 64))
		builder.WriteByte(',')
		builder.WriteString(strconv.FormatFloat(point.Longitude, 'f', 5, 64))
	}
	builder.WriteString(`)["highway"]` +
		`["highway"!~"^(proposed|construction|platform|elevator|corridor|raceway|bus_guideway|rest_area|services)$"]` +
		`["area"!="yes"];out tags geom;`)

	return builder.String()
}

// overpassResponse is the subset of the endpoint's JSON this package reads.
type overpassResponse struct {
	Remark   string            `json:"remark"`
	Elements []overpassElement `json:"elements"`
}

type overpassElement struct {
	Type     string             `json:"type"`
	Tags     map[string]string  `json:"tags"`
	Geometry []overpassPosition `json:"geometry"`
	ID       int64              `json:"id"`
}

type overpassPosition struct {
	Latitude  float64 `json:"lat"`
	Longitude float64 `json:"lon"`
}

// decodeWays turns one response into classified ways.
//
// Overpass reports a query it could not finish as a remark inside an otherwise
// valid 200 response, so a partial answer would look like a road with no
// surface anywhere. Only a remark naming an error is treated as fatal; the field
// also carries ordinary notes.
func decodeWays(body []byte) ([]Way, error) {
	var response overpassResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("surface: decoding overpass response: %w", err)
	}
	if strings.Contains(strings.ToLower(response.Remark), "error") {
		return nil, fmt.Errorf("surface: overpass reported %q", response.Remark)
	}

	ways := make([]Way, 0, len(response.Elements))
	for _, element := range response.Elements {
		if element.Type != "way" || len(element.Geometry) < 2 {
			continue
		}
		line := make([]Coordinate, 0, len(element.Geometry))
		for _, position := range element.Geometry {
			line = append(line, Coordinate{Longitude: position.Longitude, Latitude: position.Latitude})
		}
		ways = append(ways, Way{Line: line, ID: element.ID, Kind: Classify(element.Tags)})
	}

	return ways, nil
}

// chunkPoints splits geometry into runs of at most the given size, repeating the
// last point of one run as the first of the next so the queried corridor has no
// gap at the seam.
//
// The runs are made as even as the split allows rather than filled to the cap in
// turn, because the cost of a run is one whole request either way: a stage that
// divides into two full runs and a three-point remainder would otherwise pay for
// a third query that covers a few metres of road.
func chunkPoints(points []route.Point, size int) [][]route.Point {
	if len(points) <= size || size < 2 {
		return [][]route.Point{points}
	}

	segments := len(points) - 1
	perChunk := size - 1
	count := (segments + perChunk - 1) / perChunk
	base, remainder := segments/count, segments%count

	chunks := make([][]route.Point, 0, count)
	start := 0
	for index := range count {
		length := base
		if index < remainder {
			length++
		}
		chunks = append(chunks, points[start:start+length+1])
		start += length
	}

	return chunks
}

// decimate reduces geometry to the fewest points that still describe the same
// corridor.
//
// Because the around filter buffers the polyline rather than its vertices, the
// query only needs enough points to keep the straight lines between them within
// the tolerance of the route. Douglas-Peucker keeps exactly those: a straight
// road collapses to its ends while a switchback keeps every bend. Sending the
// stored geometry unchanged would make the query several times larger for the
// same answer, and it is the vertex count that Overpass struggles with.
//
// Sampling at a fixed interval instead would be simpler and worse. An interval
// coarse enough to be worth applying cuts the corners, and the corridor then
// leaves the route exactly where the route is turning — which is where a lane
// changes surface, and where the ways run closest together.
func decimate(points []route.Point, toleranceMetres float64) []route.Point {
	if len(points) <= 2 {
		return points
	}

	projection := newProjection(points[0].Longitude, points[0].Latitude)
	east := make([]float64, len(points))
	north := make([]float64, len(points))
	for index := range points {
		east[index], north[index] = projection.project(points[index].Longitude, points[index].Latitude)
	}

	keep := make([]bool, len(points))
	keep[0], keep[len(points)-1] = true, true
	simplify(east, north, keep, toleranceMetres)

	kept := make([]route.Point, 0, len(points))
	for index := range points {
		if keep[index] {
			kept = append(kept, points[index])
		}
	}

	return kept
}

// simplify marks the vertices Douglas-Peucker keeps, working through an explicit
// stack of spans so a long stage cannot recurse deeply enough to matter.
func simplify(east, north []float64, keep []bool, toleranceMetres float64) {
	type span struct {
		from int
		to   int
	}

	stack := []span{{from: 0, to: len(keep) - 1}}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		edge := segment{
			startEast:  east[current.from],
			startNorth: north[current.from],
			endEast:    east[current.to],
			endNorth:   north[current.to],
		}
		worst, worstIndex := toleranceMetres, -1
		for index := current.from + 1; index < current.to; index++ {
			if distance := edge.distanceTo(east[index], north[index]); distance > worst {
				worst, worstIndex = distance, index
			}
		}
		if worstIndex < 0 {
			continue
		}

		keep[worstIndex] = true
		stack = append(stack, span{from: current.from, to: worstIndex}, span{from: worstIndex, to: current.to})
	}
}
