// Package route owns validated source route-stage values.
package route

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

// earthRadiusMetres is the spherical Earth model shared with the elevation
// resampler so route lengths agree wherever they are computed.
const earthRadiusMetres = 6_371_000.0

// Key is the stable identity of one VeloPlanner route stage.
type Key struct {
	routeID    int64
	stageOrder int
}

// RouteID returns the immutable VeloPlanner route identifier.
func (k Key) RouteID() int64 {
	return k.routeID
}

// StageOrder returns the one-based order of the stage within its route.
func (k Key) StageOrder() int {
	return k.stageOrder
}

// ExternalID returns the deterministic Wahoo external identifier for the stage.
func (k Key) ExternalID() string {
	return fmt.Sprintf("domestique:veloplanner:%d:stage:%d", k.routeID, k.stageOrder)
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
	RouteName      string
	StageName      string
	SourceRevision string
	ContentHash    string
	Bounds         Bounds
	DistanceMetres float64
	RouteID        int64
	PointCount     int
	StageOrder     int
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
	routeID int64,
	stageOrder int,
	revision string,
	routeName string,
	stageName string,
	geometry []Point,
	contentHash string,
) (Stage, error) {
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
		key:         Key{routeID: routeID, stageOrder: stageOrder},
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
