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
	"slices"
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
	// Straight draws the leg arriving here as a straight line instead of
	// routing it. Ignored on the first waypoint.
	Straight bool
}

// Nogo is a circle the route keeps out of. The engine drops one that holds a
// waypoint, since the route must reach it.
type Nogo struct {
	Longitude    float64
	Latitude     float64
	RadiusMetres float64
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

// Answer is the engine's route: one point per vertex of the snapped line, with
// an elevation where the engine supplied one, and the ways it runs along.
type Answer struct {
	Points []route.Point
	// Ways are the line's stretches in order, as the engine reports them. Empty
	// where its answer carries no such table, which costs the route nothing.
	Ways []Way
	// Turns are the engine's turn instructions along the line, in order. Empty
	// where it gave none it could be read for.
	Turns []Turn
}

// Turn is one of the engine's turn instructions: the vertex of Points it sits
// on, and what it asks for there.
type Turn struct {
	Turn  route.Turn
	Index int
	// Exit is the roundabout exit to take; zero for any other turn.
	Exit int
}

// Way is one stretch of an answer: where along the line it ends, by the
// engine's own reckoning, and the OSM tags of the way under it.
type Way struct {
	Tags      map[string]string
	EndMetres float64
}

// Route asks the engine for the snapped line over waypoints for profile.
func (c *Client) Route(
	ctx context.Context, waypoints []Waypoint, profile string, nogos []Nogo,
) (answer Answer, err error) {
	if len(waypoints) < 2 {
		return Answer{}, &Error{Category: FailureRefused}
	}

	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := *c.baseURL
	endpoint.Path = "/brouter"
	query := url.Values{
		"lonlats":        {lonlats(waypoints)},
		"profile":        {profile},
		"alternativeidx": {"0"},
		"format":         {"geojson"},
		// Any mode above one adds the voicehints table the turns are read from.
		"timode": {"3"},
	}
	if straight := straightLegs(waypoints); straight != "" {
		query.Set("straight", straight)
	}
	if len(nogos) > 0 {
		query.Set("nogos", nogoList(nogos))
	}
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), http.NoBody)
	if err != nil {
		return Answer{}, fmt.Errorf("brouter: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/vnd.geo+json, application/json")

	response, err := c.client.Do(request)
	if err != nil {
		return Answer{}, &Error{Category: FailureUnreachable}
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	// Classified before the body is read: only a 200 is worth the cost and the
	// exposure of parsing; every other status is closed unread.
	switch {
	case response.StatusCode >= http.StatusInternalServerError:
		return Answer{}, &Error{Category: FailureEngine, Status: response.StatusCode}
	case response.StatusCode >= http.StatusBadRequest:
		return Answer{}, &Error{Category: FailureRefused, Status: response.StatusCode}
	case response.StatusCode != http.StatusOK:
		return Answer{}, &Error{Category: FailureResponse, Status: response.StatusCode}
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return Answer{}, &Error{Category: FailureUnreachable, Status: response.StatusCode}
	}
	if len(body) > maximumBodyBytes {
		return Answer{}, &Error{Category: FailureResponse, Status: response.StatusCode}
	}

	points, parseErr := parseGeometry(body, response.StatusCode)
	if parseErr != nil {
		return Answer{}, parseErr
	}

	return Answer{Points: points, Ways: parseWays(body), Turns: parseTurns(body, len(points))}, nil
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

// straightLegs names the legs to draw straight. The engine's index is the
// waypoint a leg leaves, so a waypoint reached in a straight line names the
// one before it.
func straightLegs(waypoints []Waypoint) string {
	var legs []string
	for index := 1; index < len(waypoints); index++ {
		if waypoints[index].Straight {
			legs = append(legs, strconv.Itoa(index-1))
		}
	}

	return strings.Join(legs, ",")
}

// nogoList formats circles as the engine's pipe-separated lon,lat,radius list.
func nogoList(nogos []Nogo) string {
	parts := make([]string, len(nogos))
	for index, nogo := range nogos {
		parts[index] = strconv.FormatFloat(nogo.Longitude, 'f', -1, 64) + "," +
			strconv.FormatFloat(nogo.Latitude, 'f', -1, 64) + "," +
			strconv.FormatFloat(math.Round(nogo.RadiusMetres), 'f', 0, 64)
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

// engineTurn maps one of the engine's voice-hint commands to a turn. Leaving
// the route, beeline stretches and the end point name no turn.
func engineTurn(command int) (route.Turn, bool) {
	switch command {
	case 1:
		return route.TurnStraight, true
	case 2:
		return route.TurnLeft, true
	case 3:
		return route.TurnSlightLeft, true
	case 4:
		return route.TurnSharpLeft, true
	case 5:
		return route.TurnRight, true
	case 6:
		return route.TurnSlightRight, true
	case 7:
		return route.TurnSharpRight, true
	case 8, 17:
		return route.TurnKeepLeft, true
	case 9, 18:
		return route.TurnKeepRight, true
	case 10, 11, 15:
		return route.TurnUTurn, true
	case 13, 14:
		return route.TurnRoundabout, true
	default:
		return "", false
	}
}

// turnsAnswer reads the voicehints table: one row per instruction, holding
// the vertex index, the command and the roundabout exit, then fields unused.
type turnsAnswer struct {
	Features []struct {
		Properties struct {
			VoiceHints [][]float64 `json:"voicehints"`
		} `json:"properties"`
	} `json:"features"`
}

// parseTurns reads the engine's turn instructions. A row it cannot read, or
// one naming a vertex outside the line, is left out rather than failing a
// route whose line is sound.
func parseTurns(body []byte, points int) []Turn {
	var decoded turnsAnswer
	if json.Unmarshal(body, &decoded) != nil || len(decoded.Features) == 0 {
		return nil
	}
	var turns []Turn
	for _, row := range decoded.Features[0].Properties.VoiceHints {
		if len(row) < 3 {
			continue
		}
		index, command, exit := int(row[0]), int(row[1]), int(row[2])
		turn, known := engineTurn(command)
		if !known || index < 0 || index >= points || float64(index) != row[0] {
			continue
		}
		if turn != route.TurnRoundabout || exit < 0 {
			exit = 0
		}
		turns = append(turns, Turn{Turn: turn, Index: index, Exit: exit})
	}

	return turns
}

// messagesAnswer is the other part of the answer this adapter reads: the table
// of the line's stretches, whose first row names its columns.
type messagesAnswer struct {
	Features []struct {
		Properties struct {
			Messages [][]string `json:"messages"`
		} `json:"properties"`
	} `json:"features"`
}

// parseWays reads the stretches of the line and the tags under each. A table
// it cannot read yields none rather than failing a route whose line is sound.
func parseWays(body []byte) []Way {
	var decoded messagesAnswer
	if json.Unmarshal(body, &decoded) != nil || len(decoded.Features) == 0 {
		return nil
	}
	rows := decoded.Features[0].Properties.Messages
	if len(rows) < 2 {
		return nil
	}
	distance, tagged := slices.Index(rows[0], "Distance"), slices.Index(rows[0], "WayTags")
	if distance < 0 || tagged < 0 {
		return nil
	}
	ways := make([]Way, 0, len(rows)-1)
	along := 0.0
	for _, row := range rows[1:] {
		if len(row) <= max(distance, tagged) {
			return nil
		}
		metres, err := strconv.ParseFloat(row[distance], 64)
		if err != nil || !(metres >= 0) || math.IsInf(metres, 1) {
			return nil
		}
		along += metres
		ways = append(ways, Way{EndMetres: along, Tags: parseTags(row[tagged])})
	}

	return ways
}

// parseTags reads the engine's space-separated key=value list.
func parseTags(value string) map[string]string {
	tags := make(map[string]string)
	for pair := range strings.FieldsSeq(value) {
		if key, tagValue, found := strings.Cut(pair, "="); found {
			tags[key] = tagValue
		}
	}

	return tags
}

// validCoordinate reports whether value is a finite number within [min, max].
func validCoordinate(value, minimum, maximum float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= minimum && value <= maximum
}
