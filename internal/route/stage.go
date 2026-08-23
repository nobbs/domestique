// Package route owns validated source route-stage values.
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
// matches the elevation normalizer's median window, so a gradient reported here
// describes the same scale of terrain the exported profile preserves.
const gradientWindowMetres = 100.0

// Provider names which upstream source issued a stage's route ID. It is the
// shared vocabulary a stage identity is named against: two providers may issue
// the same numeric route ID, so the pair alone is not unique once a second
// provider exists.
type Provider string

// ProviderVeloPlanner is the only provider a stage has ever come from until a
// second one exists. Naming it does not change what a VeloPlanner stage's
// ExternalID renders as.
const ProviderVeloPlanner Provider = "veloplanner"

// ProviderKomoot names the second source the webui and its demo library are
// built to distinguish. No adapter reads Komoot yet — that is a second
// source's own delivery — so today this value only ever reaches a stage
// through the synthetic demo library.
const ProviderKomoot Provider = "komoot"

// Key is the stable identity of one source route stage.
type Key struct {
	provider   Provider
	routeID    int64
	stageOrder int
}

// NewKey builds a stage identity from its parts, for a caller that has not
// constructed a Stage — a stored mapping row, or a served address.
func NewKey(provider Provider, routeID int64, stageOrder int) Key {
	return Key{provider: provider, routeID: routeID, stageOrder: stageOrder}
}

// Provider returns which upstream source issued the stage's route ID.
func (k Key) Provider() Provider {
	return k.provider
}

// RouteID returns the immutable source route identifier.
func (k Key) RouteID() int64 {
	return k.routeID
}

// StageOrder returns the one-based order of the stage within its route.
func (k Key) StageOrder() int {
	return k.stageOrder
}

// ExternalID returns the deterministic Wahoo external identifier for the
// stage. A VeloPlanner stage renders exactly as it always has; a later
// provider gets its own segment in the same grammar. This is the only place
// that renders an external ID — the string that proves ownership before a
// delete, so nothing else may format it independently.
func (k Key) ExternalID() string {
	return externalIDPrefix + fmt.Sprintf("%s:%d:stage:%d", k.provider, k.routeID, k.stageOrder)
}

// externalIDPrefix is what every external ID this service issues begins with,
// and the whole of what distinguishes a route it owns from one it must not
// touch.
const externalIDPrefix = "domestique:"

// OwnsExternalID reports whether an external ID read back from a destination
// was issued by this service.
//
// It lives beside ExternalID because the two have to agree: the prefix is the
// entire evidence of ownership, and ownership is what stands between a
// reconciliation and somebody's hand-made route. A destination route carrying
// no external ID, or another tool's, is not ours to update or delete.
func OwnsExternalID(externalID string) bool {
	return strings.HasPrefix(externalID, externalIDPrefix)
}

// Point is one geographic point in a route-stage geometry.
type Point struct {
	Elevation *float64
	Longitude float64
	Latitude  float64
}

// Bounds is the axis-aligned geographic extent of a route-stage geometry.
type Bounds struct {
	MinLongitude float64
	MinLatitude  float64
	MaxLongitude float64
	MaxLatitude  float64
}

// Summary is stored, display-oriented metadata for one source stage. It is a
// read model shared between the persistence adapter and the HTTP layer so
// neither has to know the other's types, and it deliberately carries no
// geometry.
type Summary struct {
	// MovingSeconds is the predicted moving time from internal/ridemodel, nil
	// when no coefficient file is configured, the stage has no usable
	// elevation, or nothing has predicted this exact geometry yet.
	MovingSeconds      *float64
	SourceRevision     string
	StageName          string
	Provider           Provider
	ContentHash        string
	RouteName          string
	Bounds             Bounds
	DistanceMetres     float64
	AscentMetres       float64
	MaxGradientPercent float64
	RouteID            int64
	PointCount         int
	StageOrder         int
}

// Title returns the device-facing title for the summarised stage.
func (s *Summary) Title() string {
	if s.StageName == "" {
		return s.RouteName
	}

	return s.RouteName + " — " + s.StageName
}

// Stage is a validated, immutable source route stage.
type Stage struct {
	revision    string
	routeName   string
	stageName   string
	contentHash string
	geometry    []Point
	key         Key
}

// NewStage validates and copies a source route stage.
func NewStage(
	provider Provider,
	routeID int64,
	stageOrder int,
	revision string,
	routeName string,
	stageName string,
	geometry []Point,
	contentHash string,
) (Stage, error) {
	if provider == "" {
		return Stage{}, errors.New("provider is required")
	}
	if routeID <= 0 {
		return Stage{}, errors.New("route ID must be positive")
	}
	if stageOrder <= 0 {
		return Stage{}, errors.New("stage order must be positive")
	}
	if revision == "" {
		return Stage{}, errors.New("source revision is required")
	}
	if contentHash == "" {
		return Stage{}, errors.New("content hash is required")
	}
	if len(geometry) < 2 {
		return Stage{}, errors.New("route stage requires at least two points")
	}

	copied := slices.Clone(geometry)
	for index, point := range copied {
		if err := validatePoint(point); err != nil {
			return Stage{}, fmt.Errorf("geometry point %d: %w", index, err)
		}
		if point.Elevation != nil {
			elevation := *point.Elevation
			copied[index].Elevation = &elevation
		}
	}

	return Stage{
		key:         Key{provider: provider, routeID: routeID, stageOrder: stageOrder},
		revision:    revision,
		routeName:   routeName,
		stageName:   stageName,
		geometry:    copied,
		contentHash: contentHash,
	}, nil
}

// Key returns the stable identity of the stage.
func (s *Stage) Key() Key {
	return s.key
}

// Revision returns the source revision for change detection.
func (s *Stage) Revision() string {
	return s.revision
}

// RouteName returns the mutable source route title.
func (s *Stage) RouteName() string {
	return s.routeName
}

// StageName returns the mutable source stage title.
func (s *Stage) StageName() string {
	return s.stageName
}

// Title returns the device-facing title for the source stage.
func (s *Stage) Title() string {
	if s.stageName == "" {
		return s.routeName
	}

	return s.routeName + " — " + s.stageName
}

// ContentHash returns the deterministic stage-content hash.
func (s *Stage) ContentHash() string {
	return s.contentHash
}

// Geometry returns a defensive copy of the validated stage geometry.
func (s *Stage) Geometry() []Point {
	points := slices.Clone(s.geometry)
	for index, point := range points {
		if point.Elevation != nil {
			elevation := *point.Elevation
			points[index].Elevation = &elevation
		}
	}

	return points
}

// DistanceMetres returns the cumulative great-circle length of the stage
// geometry. A stage with fewer than two points has zero length.
func (s *Stage) DistanceMetres() float64 {
	total := 0.0
	for index := 1; index < len(s.geometry); index++ {
		total += haversineMetres(s.geometry[index-1], s.geometry[index])
	}

	return total
}

// Bounds returns the extent of the stage geometry for map framing. A stage
// without geometry reports the zero value.
//
// The extent is a plain axis-aligned box. A stage crossing the antimeridian
// would report a box spanning most of the globe rather than the short way
// around; that is accepted because the source library is regional, and the
// consequence is a wide initial viewport rather than incorrect geometry.
func (s *Stage) Bounds() Bounds {
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

// ElevationGainMetres returns the total climbing in the stage profile.
//
// It sums the positive steps of the profile as stored. That is only meaningful
// on a smoothed profile: raw satellite altitude wanders by a metre or two per
// sample, and summing that noise over thousands of points inflates the total
// badly. Callers are expected to store the device-export profile, which has
// already had its spikes removed.
//
// A stage whose points do not all carry elevation has no profile to measure and
// reports zero.
func (s *Stage) ElevationGainMetres() float64 {
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

// MaxGradientPercent returns the steepest sustained gradient in the profile.
//
// Gradient is measured across a window of at least gradientWindowMetres rather
// than between adjacent points, because the gradient between two points a few
// metres apart is dominated by altitude error rather than by terrain. The
// window is what makes the number correspond to something a rider would
// recognise as a climb.
//
// A stage without a complete profile, or shorter than one window, reports zero.
func (s *Stage) MaxGradientPercent() float64 {
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
		// Advance the trailing edge while the window stays wide enough.
		//
		// trailing+1 stays in range without a bound check: distances is
		// non-decreasing, so once trailing+1 reaches leading the difference is
		// zero, which is below the window, and the loop stops. The trailing
		// edge therefore never catches the leading one.
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

func (s *Stage) hasCompleteElevation() bool {
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

// haversineMetres returns the great-circle distance between two points. It
// matches the spherical model the elevation resampler uses so distances agree
// across the service.
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
