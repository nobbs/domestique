package main

import (
	"sort"

	"github.com/nobbs/domestique/internal/elevation"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// heldOutFraction is how much of a group's rides, by date, are withheld from
// the fit and reserved for validation. The issue does not name a ratio, only
// that the split must be chronological — a random split would leak a
// fitness trend across two years and flatter the result.
const heldOutFraction = 0.2

// maxMissingGeometryFraction is how much of a ride's moving time may come
// from samples missing altitude or position (a GPS/barometer dropout)
// before the ride is unscorable here. A short dropout barely shifts the
// prediction, but a ride depending on one for a meaningful share of its
// moving time would compare a partial-ride prediction against rides.csv's
// full moving_seconds, biasing the held-out error.
const maxMissingGeometryFraction = 0.05

// splitByDate orders a group's rides chronologically and returns the earlier
// rides to fit against and the later ones to validate against — never a
// random split, per the issue's own stated reason.
func splitByDate(rides []rideRow) (train, heldOut []rideRow) {
	sorted := make([]rideRow, len(rides))
	copy(sorted, rides)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Date.Before(sorted[j].Date) })

	heldOutCount := int(float64(len(sorted)) * heldOutFraction)
	if heldOutCount == 0 && len(sorted) > 1 {
		heldOutCount = 1
	}
	splitAt := len(sorted) - heldOutCount

	return sorted[:splitAt], sorted[splitAt:]
}

// predictedMovingSeconds predicts a ride's moving time from fixed-mass,
// fixed-power physics alone — physics.go's own copy of the physics branch
// internal/ridemodel.Predict ran before #240 made that function inherently
// hybrid; see physics.go's doc comment for why this package keeps a
// short-lived copy rather than calling internal/ridemodel directly.
//
// The ride is first passed through the production elevation normalizer, so
// validation sees the same stored profile a deployed route does rather than a
// noisier recorded-altitude surrogate.
//
// A moving sample missing altitude or position (a GPS/barometer dropout) is
// dropped from the point sequence rather than making the ride unscorable
// outright — see maxMissingGeometryFraction for why a small amount of this
// is tolerated.
func predictedMovingSeconds(samples []sampleRow, result *fitResult, config coefficientsConfig, driveEfficiency, powerWatts float64) float64 {
	stage, kinds, ok := normalizedRideStage(samples)
	if !ok {
		return 0
	}

	crrBySurface := fullCrrBySurface(result)
	points := stage.Geometry()
	crrPerSegment := make([]float64, max(0, len(points)-1))
	for i := range crrPerSegment {
		kind := surface.KindAsphalt
		if i < len(kinds) && kinds[i] != surface.KindUnknown {
			kind = kinds[i]
		}
		crrPerSegment[i] = crrBySurface[kind]
	}

	config.DriveEfficiency = driveEfficiency

	return physicsOnlyMovingSeconds(points, crrPerSegment, result.MassKG, powerWatts, result.CdA, config)
}

// normalizedRideStage converts a ridden trace into the same elevation profile
// production stores and predicts: the normalizer preserves the GPS line while
// resampling and median-filtering its altitude channel.
func normalizedRideStage(samples []sampleRow) (route.Stage, []surface.Kind, bool) {
	points := make([]route.Point, 0, len(samples))
	kinds := make([]surface.Kind, 0, len(samples))
	var movingSeconds, missingGeometrySeconds float64
	for i := range samples {
		s := &samples[i]
		if !s.Moving {
			continue
		}
		movingSeconds += s.DeltaSeconds
		if !s.HasAltitude || !s.HasPosition {
			missingGeometrySeconds += s.DeltaSeconds

			continue
		}
		altitude := s.AltitudeM
		points = append(points, route.Point{Latitude: s.Latitude, Longitude: s.Longitude, Elevation: &altitude})
		kinds = append(kinds, s.Surface)
	}
	if movingSeconds > 0 && missingGeometrySeconds/movingSeconds > maxMissingGeometryFraction {
		return route.Stage{}, nil, false
	}
	stage, err := route.NewStage(
		route.ProviderVeloPlanner, 1, 1, "benchmark", "benchmark", "", points, "benchmark",
	)
	if err != nil {
		return route.Stage{}, nil, false
	}
	normalized, err := elevation.New().Process(&stage)
	if err != nil {
		return route.Stage{}, nil, false
	}

	return normalized, kinds, true
}

// baselineMovingSeconds is the trivial predictor the issue asks the model to
// beat: distance at a flat-riding mean speed, plus ascent at a mean climbing
// rate (VAM), both estimated from the same training rides the physics model
// was fitted against — never from the held-out rides being scored.
func baselineMovingSeconds(samples []sampleRow, flatSpeedMPS, vamMetresPerHour, climbThresholdPercent float64) float64 {
	var flatDistance, ascent float64
	for i := range samples {
		if !samples[i].Moving {
			continue
		}
		if samples[i].GradientPercent < climbThresholdPercent {
			flatDistance += samples[i].IntervalDistance
		} else if samples[i].HasAltitude {
			ascent += climbRise(samples, i)
		}
	}

	var seconds float64
	if flatSpeedMPS > 0 {
		seconds += flatDistance / flatSpeedMPS
	}
	if vamMetresPerHour > 0 {
		seconds += (ascent / vamMetresPerHour) * 3600
	}

	return seconds
}

// climbRise is one sample's own rise, backed out of its grade and distance
// rather than a raw altitude delta, so it agrees with the same
// windowed-gradient the rest of this package already uses.
func climbRise(samples []sampleRow, i int) float64 {
	return samples[i].IntervalDistance * samples[i].GradientPercent / 100
}

// baselineCoefficients estimates the trivial baseline's own two constants
// from a set of training rides: the mean speed of flat riding, and the mean
// vertical ascent rate of sustained climbing.
func baselineCoefficients(
	samplesByRide map[string][]sampleRow, rideIDs map[string]bool, climbThresholdPercent float64,
) (flatSpeedMPS, vamMetresPerHour float64) {
	var flatDistance, flatSeconds, ascent, climbSeconds float64
	for rideID, samples := range samplesByRide {
		if !rideIDs[rideID] {
			continue
		}
		for i := range samples {
			if !samples[i].Moving {
				continue
			}
			if samples[i].GradientPercent < climbThresholdPercent {
				flatDistance += samples[i].IntervalDistance
				flatSeconds += samples[i].DeltaSeconds
			} else if samples[i].HasAltitude {
				ascent += climbRise(samples, i)
				climbSeconds += samples[i].DeltaSeconds
			}
		}
	}

	if flatSeconds > 0 {
		flatSpeedMPS = flatDistance / flatSeconds
	}
	if climbSeconds > 0 {
		vamMetresPerHour = ascent / (climbSeconds / 3600)
	}

	return flatSpeedMPS, vamMetresPerHour
}

// heldOutValidation is the report's own held-out figures: mean absolute
// percentage error of the physics model and of the trivial baseline, over
// the same held-out rides, so the model's added complexity is visibly
// earning its place or not.
type heldOutValidation struct {
	Rides       int
	ModelMAE    float64
	BaselineMAE float64
}

func validateHeldOut(
	heldOutRideIDs map[string]bool, samplesByRide map[string][]sampleRow,
	actualMovingSeconds map[string]float64, result *fitResult, config coefficientsConfig,
	flatSpeedMPS, vamMetresPerHour, climbThresholdPercent float64,
) heldOutValidation {
	var modelErrors, baselineErrors []float64
	for rideID := range heldOutRideIDs {
		samples, ok := samplesByRide[rideID]
		actual, hasActual := actualMovingSeconds[rideID]
		if !ok || !hasActual || actual <= 0 {
			continue
		}

		// Scored only when both predictors produce a value for this ride: the
		// two MAEs are meant to be directly comparable, over the same rides,
		// not each over whatever subset happened to have a prediction.
		predicted := predictedMovingSeconds(samples, result, config, config.DriveEfficiency, result.PowerWatts)
		baseline := baselineMovingSeconds(samples, flatSpeedMPS, vamMetresPerHour, climbThresholdPercent)
		if predicted <= 0 || baseline <= 0 {
			continue
		}
		modelErrors = append(modelErrors, absPercentError(predicted, actual))
		baselineErrors = append(baselineErrors, absPercentError(baseline, actual))
	}

	return heldOutValidation{
		Rides:       len(modelErrors),
		ModelMAE:    meanOf(modelErrors),
		BaselineMAE: meanOf(baselineErrors),
	}
}

func absPercentError(predicted, actual float64) float64 {
	diff := predicted - actual
	if diff < 0 {
		diff = -diff
	}

	return 100 * diff / actual
}

func meanOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}
