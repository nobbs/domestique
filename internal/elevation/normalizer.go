// Package elevation normalizes route elevation profiles for device export.
package elevation

import (
	"fmt"
	"math"
	"sort"

	"github.com/nobbs/domestique/internal/route"
)

const (
	sampleIntervalMetres = 25.0
	medianWindowMetres   = 100.0
	earthRadiusMetres    = 6_371_000.0
)

// Normalizer resamples fully-elevated stages and removes isolated altitude
// spikes with a centred moving median. Stages with incomplete elevation are
// preserved because they do not provide a complete profile to normalize.
type Normalizer struct{}

// New creates a normalizer using the fixed device-export elevation policy.
func New() *Normalizer {
	return &Normalizer{}
}

// Process returns a stage with a normalized elevation profile while retaining
// its source identity, revision, title, and source content hash.
func (n *Normalizer) Process(stage *route.Stage) (route.Stage, error) {
	if stage == nil {
		return route.Stage{}, fmt.Errorf("elevation: route stage is required")
	}
	geometry := stage.Geometry()
	if !hasCompleteElevation(geometry) {
		return *stage, nil
	}

	resampled := resample(geometry)
	applyMovingMedian(resampled)

	processed, err := route.NewStage(
		stage.Key().RouteID(),
		stage.Key().StageOrder(),
		stage.Revision(),
		stage.RouteName(),
		stage.StageName(),
		resampled,
		stage.ContentHash(),
	)
	if err != nil {
		return route.Stage{}, fmt.Errorf("elevation: creating normalized stage: %w", err)
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

func resample(points []route.Point) []route.Point {
	result := []route.Point{copyPoint(points[0])}
	nextSample := sampleIntervalMetres
	distanceSoFar := 0.0
	for index := 1; index < len(points); index++ {
		previous, current := points[index-1], points[index]
		segmentDistance := haversine(previous, current)
		for segmentDistance > 0 && distanceSoFar+segmentDistance >= nextSample {
			ratio := (nextSample - distanceSoFar) / segmentDistance
			result = append(result, interpolate(previous, current, ratio))
			nextSample += sampleIntervalMetres
		}
		distanceSoFar += segmentDistance
	}

	last := points[len(points)-1]
	if resultLast := result[len(result)-1]; resultLast.Latitude != last.Latitude || resultLast.Longitude != last.Longitude {
		result = append(result, copyPoint(last))
	}

	return result
}

func applyMovingMedian(points []route.Point) {
	radius := int(medianWindowMetres / sampleIntervalMetres / 2)
	elevations := make([]float64, len(points))
	for index, point := range points {
		elevations[index] = *point.Elevation
	}
	for index := range points {
		start, end := max(0, index-radius), min(len(elevations), index+radius+1)
		window := append([]float64(nil), elevations[start:end]...)
		sort.Float64s(window)
		elevation := window[len(window)/2]
		points[index].Elevation = &elevation
	}
}

func copyPoint(point route.Point) route.Point {
	elevation := *point.Elevation
	return route.Point{Longitude: point.Longitude, Latitude: point.Latitude, Elevation: &elevation}
}

func interpolate(left, right route.Point, ratio float64) route.Point {
	elevation := *left.Elevation + ratio*(*right.Elevation-*left.Elevation)
	return route.Point{
		Longitude: left.Longitude + ratio*(right.Longitude-left.Longitude),
		Latitude:  left.Latitude + ratio*(right.Latitude-left.Latitude),
		Elevation: &elevation,
	}
}

func haversine(left, right route.Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
