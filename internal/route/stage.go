// Package route owns validated source route-stage values.
package route

import (
	"errors"
	"fmt"
	"math"
	"slices"
)

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
