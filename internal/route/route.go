// Package route owns validated route values. A route is one ride's worth of
// geometry: a whole Komoot tour, or one stage of a VeloPlanner source route.
package route

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// earthRadiusMetres is the spherical Earth model shared with the elevation
// resampler so route lengths agree wherever they are computed.
const earthRadiusMetres = 6_371_000.0

// gradientWindowMetres is the shortest span a gradient is measured over. It
// matches the elevation normalizer's median window.
const gradientWindowMetres = 100.0

// Provider names which upstream issued a route's source route ID. Two providers
// may issue the same numeric ID, so that ID alone is not unique.
type Provider string

// ProviderVeloPlanner is the only provider a route comes from today. Naming it
// does not change what a VeloPlanner route's ExternalID renders as.
const ProviderVeloPlanner Provider = "veloplanner"

// ProviderKomoot names the second source the webui and its demo library
// distinguish. No adapter reads Komoot yet; only the demo library sets it.
const ProviderKomoot Provider = "komoot"

// Key is the stable identity of one route.
type Key struct {
	provider      Provider
	sourceRouteID int64
	stageOrder    int
}

// NewKey builds a route identity from its parts, for a caller that has not
// constructed a Route — a stored mapping row, or a served address.
func NewKey(provider Provider, sourceRouteID int64, stageOrder int) Key {
	return Key{provider: provider, sourceRouteID: sourceRouteID, stageOrder: stageOrder}
}

// Provider returns which upstream source issued the route's source route ID.
func (k Key) Provider() Provider {
	return k.provider
}

// SourceRouteID returns the immutable source route identifier.
func (k Key) SourceRouteID() int64 {
	return k.sourceRouteID
}

// StageOrder returns the route's one-based order within its source route.
func (k Key) StageOrder() int {
	return k.stageOrder
}

// ExternalID returns the deterministic Wahoo external identifier for the route,
// and is the only place one is rendered: it is the string that proves ownership
// before a delete. The `stage` segment is frozen — ownership is matched against
// it, so respelling it would orphan the library rather than rename anything.
func (k Key) ExternalID() string {
	return externalIDPrefix + fmt.Sprintf("%s:%d:stage:%d", k.provider, k.sourceRouteID, k.stageOrder)
}

// externalIDPrefix is what every external ID this service issues begins with,
// and the whole of what distinguishes a route it owns from one it must not touch.
const externalIDPrefix = "domestique:"

// OwnsExternalID reports whether an external ID read back from a destination was
// issued by this service. The prefix is the entire evidence of ownership: a
// destination route carrying none, or another tool's, is not ours to touch.
func OwnsExternalID(externalID string) bool {
	return strings.HasPrefix(externalID, externalIDPrefix)
}

// Point is one geographic point in a route geometry.
type Point struct {
	Elevation *float64
	Longitude float64
	Latitude  float64
}

// Bounds is the axis-aligned geographic extent of a route geometry.
type Bounds struct {
	MinLongitude float64
	MinLatitude  float64
	MaxLongitude float64
	MaxLatitude  float64
}

// Summary is stored, display-oriented metadata for one route: a read model
// shared between the persistence adapter and the HTTP layer. It carries no
// geometry.
type Summary struct {
	// MovingSeconds is the predicted moving time, nil when no coefficient file is
	// configured, the route has no usable elevation, or nothing has predicted it.
	MovingSeconds      *float64
	SourceRevision     string
	RouteName          string
	Provider           Provider
	ContentHash        string
	SourceRouteName    string
	Bounds             Bounds
	DistanceMetres     float64
	AscentMetres       float64
	MaxGradientPercent float64
	SourceRouteID      int64
	PointCount         int
	StageOrder         int
}

// Title returns the device-facing title for the summarised route.
func (s *Summary) Title() string {
	if s.RouteName == "" {
		return s.SourceRouteName
	}

	return s.SourceRouteName + " — " + s.RouteName
}

// Route is a validated, immutable route.
type Route struct {
	revision        string
	sourceRouteName string
	routeName       string
	contentHash     string
	geometry        []Point
	key             Key
}

// NewRoute validates and copies a route.
func NewRoute(
	provider Provider,
	sourceRouteID int64,
	stageOrder int,
	revision string,
	sourceRouteName string,
	routeName string,
	geometry []Point,
	contentHash string,
) (Route, error) {
	if provider == "" {
		return Route{}, errors.New("provider is required")
	}
	if sourceRouteID <= 0 {
		return Route{}, errors.New("source route ID must be positive")
	}
	if stageOrder <= 0 {
		return Route{}, errors.New("stage order must be positive")
	}
	if revision == "" {
		return Route{}, errors.New("source revision is required")
	}
	if contentHash == "" {
		return Route{}, errors.New("content hash is required")
	}
	if len(geometry) < 2 {
		return Route{}, errors.New("route requires at least two points")
	}

	copied := slices.Clone(geometry)
	for index, point := range copied {
		if err := validatePoint(point); err != nil {
			return Route{}, fmt.Errorf("geometry point %d: %w", index, err)
		}
		if point.Elevation != nil {
			elevation := *point.Elevation
			copied[index].Elevation = &elevation
		}
	}

	return Route{
		key:             Key{provider: provider, sourceRouteID: sourceRouteID, stageOrder: stageOrder},
		revision:        revision,
		sourceRouteName: sourceRouteName,
		routeName:       routeName,
		geometry:        copied,
		contentHash:     contentHash,
	}, nil
}

// Key returns the stable identity of the route.
func (s *Route) Key() Key {
	return s.key
}

// Revision returns the source revision for change detection.
func (s *Route) Revision() string {
	return s.revision
}

// SourceRouteName returns the mutable source route title.
func (s *Route) SourceRouteName() string {
	return s.sourceRouteName
}

// RouteName returns the mutable route title, as the source spells it.
func (s *Route) RouteName() string {
	return s.routeName
}

// Title returns the device-facing title for the route.
func (s *Route) Title() string {
	if s.routeName == "" {
		return s.sourceRouteName
	}

	return s.sourceRouteName + " — " + s.routeName
}

// ContentHash returns the deterministic route-content hash.
func (s *Route) ContentHash() string {
	return s.contentHash
}

// Geometry returns a defensive copy of the validated route geometry.
func (s *Route) Geometry() []Point {
	points := slices.Clone(s.geometry)
	for index, point := range points {
		if point.Elevation != nil {
			elevation := *point.Elevation
			points[index].Elevation = &elevation
		}
	}

	return points
}

// DistanceMetres returns the cumulative great-circle length of the route
// geometry. A route with fewer than two points has zero length.
func (s *Route) DistanceMetres() float64 {
	total := 0.0
	for index := 1; index < len(s.geometry); index++ {
		total += haversineMetres(s.geometry[index-1], s.geometry[index])
	}

	return total
}

// Bounds returns the extent of the route geometry for map framing; zero without
// geometry. A route crossing the antimeridian reports a box spanning most of the
// globe, which is a wide viewport rather than incorrect geometry.
func (s *Route) Bounds() Bounds {
	if len(s.geometry) == 0 {
		return Bounds{}
	}

	first := s.geometry[0]
	bounds := Bounds{
		MinLongitude: first.Longitude,
		MinLatitude:  first.Latitude,
		MaxLongitude: first.Longitude,
		MaxLatitude:  first.Latitude,
	}
	for _, point := range s.geometry[1:] {
		bounds.MinLongitude = min(bounds.MinLongitude, point.Longitude)
		bounds.MinLatitude = min(bounds.MinLatitude, point.Latitude)
		bounds.MaxLongitude = max(bounds.MaxLongitude, point.Longitude)
		bounds.MaxLatitude = max(bounds.MaxLatitude, point.Latitude)
	}

	return bounds
}

// ElevationGainMetres returns the total ascent in the route profile, summing the
// positive steps as stored. Only meaningful on a smoothed profile: raw satellite
// altitude noise summed over thousands of points inflates the total badly. A
// route without full elevation reports zero.
func (s *Route) ElevationGainMetres() float64 {
	if !s.hasCompleteElevation() {
		return 0
	}

	gain := 0.0
	for index := 1; index < len(s.geometry); index++ {
		step := *s.geometry[index].Elevation - *s.geometry[index-1].Elevation
		if step > 0 {
			gain += step
		}
	}

	return gain
}

// MaxGradientPercent returns the steepest sustained gradient, measured across a
// window of at least gradientWindowMetres rather than between adjacent points,
// where altitude error dominates. Zero without a complete profile.
func (s *Route) MaxGradientPercent() float64 {
	if !s.hasCompleteElevation() {
		return 0
	}

	// Cumulative distance along the route, so a window can be found by
	// scanning forward rather than re-measuring.
	distances := make([]float64, len(s.geometry))
	for index := 1; index < len(s.geometry); index++ {
		distances[index] = distances[index-1] + haversineMetres(s.geometry[index-1], s.geometry[index])
	}

	steepest := 0.0
	trailing := 0
	for leading := 1; leading < len(s.geometry); leading++ {
		// trailing+1 stays in range without a bound check: distances is non-decreasing,
		// so the difference reaches zero and the loop stops.
		for distances[leading]-distances[trailing+1] >= gradientWindowMetres {
			trailing++
		}
		span := distances[leading] - distances[trailing]
		if span < gradientWindowMetres {
			continue
		}
		rise := *s.geometry[leading].Elevation - *s.geometry[trailing].Elevation
		gradient := math.Abs(rise) / span * 100
		steepest = max(steepest, gradient)
	}

	return steepest
}

func (s *Route) hasCompleteElevation() bool {
	if len(s.geometry) < 2 {
		return false
	}
	for _, point := range s.geometry {
		if point.Elevation == nil {
			return false
		}
	}

	return true
}

// haversineMetres returns the great-circle distance between two points, on the
// same spherical model the elevation resampler uses.
func haversineMetres(left, right Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	chord := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(chord), math.Sqrt(1-chord))
}

func validatePoint(point Point) error {
	if math.IsNaN(point.Longitude) || math.IsInf(point.Longitude, 0) ||
		point.Longitude < -180 || point.Longitude > 180 {
		return errors.New("longitude is outside the valid range")
	}
	if math.IsNaN(point.Latitude) || math.IsInf(point.Latitude, 0) ||
		point.Latitude < -90 || point.Latitude > 90 {
		return errors.New("latitude is outside the valid range")
	}
	if point.Elevation != nil && (math.IsNaN(*point.Elevation) ||
		math.IsInf(*point.Elevation, 0)) {
		return errors.New("elevation is not finite")
	}

	return nil
}
