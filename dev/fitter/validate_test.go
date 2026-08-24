package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

func TestSplitByDateNeverShufflesAcrossTheBoundary(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rides := make([]rideRow, 20)
	for i := range rides {
		rides[i] = rideRow{RideID: string(rune('a' + i)), Date: start.AddDate(0, 0, i)}
	}

	train, heldOut := splitByDate(rides)
	require.NotEmpty(t, heldOut)
	for _, tr := range train {
		for _, ho := range heldOut {
			assert.True(t, tr.Date.Before(ho.Date), "a training ride must not be later than a held-out one")
		}
	}
}

// threeSamplesUpAGrade builds an eligible sample sequence climbing at a
// steady grade — enough points for ridemodel.Predict's own
// hasCompleteElevation check, and a known geometry to check the wrapper's
// output against a direct call to ridemodel.Predict with the same points.
func threeSamplesUpAGrade(gradePercent float64) []sampleRow {
	return samplesUpAGrade(gradePercent, 3)
}

func samplesUpAGrade(gradePercent float64, count int) []sampleRow {
	when := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)
	lat := 50.0
	samples := make([]sampleRow, count)
	for i := range samples {
		samples[i] = sampleRow{
			Time: when.Add(time.Duration(i) * time.Second), DeltaSeconds: 1,
			IntervalDistance: 5, Latitude: lat, Longitude: 8.0, AltitudeM: float64(i) * 5 * gradePercent / 100,
			HasAltitude: true, HasPosition: true, Moving: true,
		}
		lat += 0.00005
	}

	return samples
}

// predictedMovingSeconds must be a thin adapter over internal/ridemodel.Predict
// — this checks it produces exactly what a direct call to Predict with the
// same points, kinds and coefficients would, rather than a second
// reimplementation that happens to agree today.
func TestPredictedMovingSecondsMatchesADirectRidemodelPredictCall(t *testing.T) {
	samples := threeSamplesUpAGrade(6.0)
	result := &fitResult{CrrOverall: 0.006, CdA: 0.45, MassKG: 90.0}
	config := coefficientsConfig{DriveEfficiency: 0.975, AirDensityKGPerM3: 1.2, DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0}

	got := predictedMovingSeconds(samples, result, config, config.DriveEfficiency, 155.0)

	points := make([]route.Point, len(samples))
	kinds := make([]surface.Kind, len(samples))
	for i := range samples {
		altitude := samples[i].AltitudeM
		points[i] = route.Point{Latitude: samples[i].Latitude, Longitude: samples[i].Longitude, Elevation: &altitude}
		kinds[i] = samples[i].Surface
	}
	want, ok := ridemodel.Predict(points, kinds, ridemodel.Coefficients{
		MassKG: 90.0, PowerWatts: 155.0, DriveEfficiency: 0.975, CdAM2: 0.45, AirDensityKGPerM3: 1.2,
		DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0, CrrBySurface: fullCrrBySurface(result),
	})
	require.True(t, ok)
	assert.InDelta(t, want.MovingSeconds, got, 1e-9)
}

func TestPredictedMovingSecondsReturnsZeroWithFewerThanTwoEligibleSamples(t *testing.T) {
	samples := []sampleRow{{Moving: true, HasAltitude: true, HasPosition: true, Latitude: 50.0, Longitude: 8.0}}
	result := &fitResult{CrrOverall: 0.006, CdA: 0.45, MassKG: 90.0}
	config := coefficientsConfig{DriveEfficiency: 0.975, AirDensityKGPerM3: 1.2, DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0}

	got := predictedMovingSeconds(samples, result, config, config.DriveEfficiency, 155.0)
	assert.InDelta(t, 0.0, got, 1e-9)
}

// A ride whose moving samples miss altitude or position — a GPS/barometer
// dropout — for more than maxMissingGeometryFraction of its moving time must
// be unscorable, not just have those samples skipped from the point
// sequence: a prediction over the remaining points alone would compare a
// materially partial-ride time against the ride's full moving_seconds.
func TestPredictedMovingSecondsReturnsZeroWhenMissingGeometryExceedsTheTolerance(t *testing.T) {
	samples := threeSamplesUpAGrade(6.0)
	samples[1].HasPosition = false // 1 of 3 seconds missing, well past the 5% tolerance
	result := &fitResult{CrrOverall: 0.006, CdA: 0.45, MassKG: 90.0}
	config := coefficientsConfig{DriveEfficiency: 0.975, AirDensityKGPerM3: 1.2, DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0}

	got := predictedMovingSeconds(samples, result, config, config.DriveEfficiency, 155.0)
	assert.InDelta(t, 0.0, got, 1e-9)
}

// A brief geometry dropout — under maxMissingGeometryFraction of the ride's
// moving time — must not zero out the whole ride: real GPS/barometer
// dropouts this small are routine and barely shift the predicted time.
func TestPredictedMovingSecondsToleratesABriefGeometryDropout(t *testing.T) {
	samples := samplesUpAGrade(6.0, 25)
	samples[10].HasPosition = false // 1 of 25 seconds missing, under the 5% tolerance
	result := &fitResult{CrrOverall: 0.006, CdA: 0.45, MassKG: 90.0}
	config := coefficientsConfig{DriveEfficiency: 0.975, AirDensityKGPerM3: 1.2, DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0}

	got := predictedMovingSeconds(samples, result, config, config.DriveEfficiency, 155.0)
	assert.Greater(t, got, 0.0)
}

func TestValidateHeldOutReportsBothMAEsOverTheHeldOutRidesOnly(t *testing.T) {
	samplesByRide := map[string][]sampleRow{"heldout1": threeSamplesUpAGrade(6.0)}
	for i := range samplesByRide["heldout1"] {
		samplesByRide["heldout1"][i].RideID = "heldout1"
	}
	result := fitResult{CrrOverall: 0.006, CdA: 0.45, MassKG: 90.0, PowerWatts: 155.0}
	config := coefficientsConfig{DriveEfficiency: 0.975, AirDensityKGPerM3: 1.2, DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0}

	summary := validateHeldOut(
		map[string]bool{"heldout1": true}, samplesByRide, map[string]float64{"heldout1": 100.0},
		&result, config, 8.0, 500.0, defaultClimbThresholdPercent,
	)
	assert.Equal(t, 1, summary.Rides)
	assert.GreaterOrEqual(t, summary.ModelMAE, 0.0)
	assert.GreaterOrEqual(t, summary.BaselineMAE, 0.0)
}

// A run configured with a non-default -climb-threshold-percent must have
// its baseline split flat from climbing at that same threshold, not the
// package's own default — otherwise the baseline the model is compared
// against does not describe the run it actually did.
func TestBaselineMovingSecondsUsesTheConfiguredClimbThreshold(t *testing.T) {
	samples := []sampleRow{
		{Moving: true, HasAltitude: true, IntervalDistance: 100, DeltaSeconds: 10, GradientPercent: 3.0},
	}

	// Below a 2% threshold this sample counts as climbing (ascent), so the
	// flat-speed term contributes nothing; above a 5% threshold it counts as
	// flat, so the VAM term contributes nothing.
	asClimb := baselineMovingSeconds(samples, 8.0, 500.0, 2.0)
	asFlat := baselineMovingSeconds(samples, 8.0, 500.0, 5.0)
	assert.NotEqual(t, asClimb, asFlat)
	assert.InDelta(t, 100.0/8.0, asFlat, 1e-9)
}
