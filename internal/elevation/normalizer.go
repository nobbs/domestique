// Package elevation holds the device-export elevation policy: which interval
// and which window, applied through measure.
package elevation

import (
	"fmt"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

const (
	sampleIntervalMetres = 25.0
	medianWindowMetres   = 100.0
)

// Normalizer resamples fully-elevated routes and removes isolated altitude
// spikes with a centred moving median. Routes with incomplete elevation are
// preserved because they do not provide a complete profile to normalize.
type Normalizer struct{}

// New creates a normalizer using the fixed device-export elevation policy.
func New() *Normalizer {
	return &Normalizer{}
}

// Process returns a route with a normalized elevation profile while retaining
// its source identity, revision, title, and source content hash.
func (n *Normalizer) Process(original *route.Route) (route.Route, error) {
	if original == nil {
		return route.Route{}, fmt.Errorf("elevation: route is required")
	}
	geometry := original.Geometry()
	if !hasCompleteElevation(geometry) {
		return *original, nil
	}

	distances := route.CumulativeMetres(geometry)
	altitudes := make([]float64, len(geometry))
	for index, point := range geometry {
		altitudes[index] = *point.Elevation
	}
	// NewRoute holds every geometry to at least two points, so this cannot refuse.
	profile, _ := measure.ProfileOf(distances, altitudes)
	smoothed := profile.Resample(sampleIntervalMetres).MedianFiltered(sampleIntervalMetres, medianWindowMetres)
	for index, distance := range distances {
		elevation := smoothed.AltitudeAt(distance)
		geometry[index].Elevation = &elevation
	}

	processed, err := route.NewRoute(
		original.Key().Provider(),
		original.Key().SourceRouteID(),
		original.Key().StageOrder(),
		original.Revision(),
		original.SourceRouteName(),
		original.RouteName(),
		geometry,
		original.ContentHash(),
	)
	if err != nil {
		return route.Route{}, fmt.Errorf("elevation: creating normalized route: %w", err)
	}

	return processed, nil
}

func hasCompleteElevation(points []route.Point) bool {
	for _, point := range points {
		if point.Elevation == nil {
			return false
		}
	}

	return true
}
