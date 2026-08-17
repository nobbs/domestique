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

	profile := resampleElevations(geometry)
	applyMovingMedian(profile)
	applyElevations(geometry, profile)

	processed, err := route.NewStage(
		stage.Key().RouteID(),
		stage.Key().StageOrder(),
		stage.Revision(),
		stage.RouteName(),
		stage.StageName(),
		geometry,
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

type sample struct {
	distance  float64
	elevation float64
}

func resampleElevations(points []route.Point) []sample {
	result := []sample{{elevation: *points[0].Elevation}}
	nextSample := sampleIntervalMetres
	distanceSoFar := 0.0
	for index := 1; index < len(points); index++ {
		previous, current := points[index-1], points[index]
		segmentDistance := haversine(previous, current)
		for segmentDistance > 0 && distanceSoFar+segmentDistance >= nextSample {
			ratio := (nextSample - distanceSoFar) / segmentDistance
			result = append(result, sample{
				distance:  nextSample,
				elevation: *previous.Elevation + ratio*(*current.Elevation-*previous.Elevation),
			})
			nextSample += sampleIntervalMetres
		}
		distanceSoFar += segmentDistance
	}

	if resultLast := result[len(result)-1]; resultLast.distance != distanceSoFar {
		result = append(result, sample{distance: distanceSoFar, elevation: *points[len(points)-1].Elevation})
	}

	return result
}

func applyMovingMedian(samples []sample) {
	radius := int(medianWindowMetres / sampleIntervalMetres / 2)
	elevations := make([]float64, len(samples))
	for index, sample := range samples {
		elevations[index] = sample.elevation
	}
	for index := range samples {
		start, end := max(0, index-radius), min(len(elevations), index+radius+1)
		window := append([]float64(nil), elevations[start:end]...)
		sort.Float64s(window)
		elevation := window[len(window)/2]
		samples[index].elevation = elevation
	}
}

func applyElevations(points []route.Point, samples []sample) {
	distanceSoFar := 0.0
	sampleIndex := 0
	for index := range points {
		if index > 0 {
			distanceSoFar += haversine(points[index-1], points[index])
		}
		for sampleIndex+1 < len(samples) && samples[sampleIndex+1].distance <= distanceSoFar {
			sampleIndex++
		}
		left := samples[sampleIndex]
		if sampleIndex == len(samples)-1 {
			elevation := left.elevation
			points[index].Elevation = &elevation
			continue
		}
		right := samples[sampleIndex+1]
		ratio := (distanceSoFar - left.distance) / (right.distance - left.distance)
		elevation := left.elevation + ratio*(right.elevation-left.elevation)
		points[index].Elevation = &elevation
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
