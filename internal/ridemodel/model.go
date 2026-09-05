package ridemodel

import (
	"math"

	"github.com/nobbs/domestique/internal/route"
)

// modelVersion identifies the equation below, which no calibration can vary.
// WithFingerprint mixes it into a pair's fingerprint: bump it whenever the
// equation changes, or a stale cached duration would look as current as it did
// before the upgrade.
const modelVersion = "linear-v1"

// Result is one stage's predicted moving time: the total, and the running time at
// every point of the geometry. Two consumers need a time at an arbitrary point
// and neither can reconstruct one from a scalar.
type Result struct {
	CumulativeSeconds []float64
	MovingSeconds     float64
}

// Predict runs the forward model over one stage's geometry: distance and ascent
// priced by the calibrated coefficients, accumulated per segment so the running
// total stays aligned 1:1 with the geometry. False when the geometry has no
// usable elevation.
//
//nolint:gocritic // value param: Coefficients is immutable once loaded, and a pointer would let a caller mutate the shared instance mid-prediction.
func Predict(points []route.Point, coefficients Coefficients) (Result, bool) {
	if !hasCompleteElevation(points) {
		return Result{}, false
	}

	cumulative := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		span := route.HaversineMetres(points[index-1], points[index])
		if span <= 0 {
			// No progress along the ground: a repeated point's elevation noise
			// must not read as climbing.
			cumulative[index] = cumulative[index-1]

			continue
		}
		// The raw, unwindowed rise, because the ascent term matches
		// route.Route.ElevationGainMetres(), which is what seconds_per_ascent_m
		// was calibrated against.
		rise := *points[index].Elevation - *points[index-1].Elevation
		cumulative[index] = cumulative[index-1] +
			coefficients.SecondsPerKM*(span/1000) + coefficients.SecondsPerAscentM*math.Max(0, rise)
	}

	return Result{
		MovingSeconds:     cumulative[len(cumulative)-1],
		CumulativeSeconds: cumulative,
	}, true
}

func hasCompleteElevation(points []route.Point) bool {
	if len(points) < 2 {
		return false
	}
	for _, point := range points {
		if point.Elevation == nil {
			return false
		}
	}

	return true
}
