package main

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/surface"
)

func testCoefficients() ridemodel.Coefficients {
	return ridemodel.Coefficients{
		CalibrationCutoff: "2025-08-01",
		MassKG:            90,
		PowerWatts:        180,
		CdAM2:             0.45,
		SecondsPerKM:      140,
		SecondsPerAscentM: 4,
		CrrBySurface: map[surface.Kind]float64{
			surface.KindAsphalt:   0.012,
			surface.KindPaving:    0.012,
			surface.KindCompacted: 0.012,
			surface.KindGravel:    0.012,
			surface.KindGround:    0.012,
		},
	}
}

func TestClusterRoutesTreatsTheSameTraceInReverseAsOneRoute(t *testing.T) {
	forward := positionedSamples([][2]float64{{50.000, 8.000}, {50.002, 8.002}, {50.004, 8.004}})
	reverse := positionedSamples([][2]float64{{50.004, 8.004}, {50.002, 8.002}, {50.000, 8.000}})
	other := positionedSamples([][2]float64{{51.000, 9.000}, {51.002, 9.002}, {51.004, 9.004}})
	rides := []rideRow{{RideID: "forward"}, {RideID: "reverse"}, {RideID: "other"}}

	clusters, count, repeated, largest := clusterRoutes(rides, map[string][]sampleRow{
		"forward": forward, "reverse": reverse, "other": other,
	}, defaultRouteCellDegrees, defaultRouteJaccardThreshold)

	assert.Equal(t, clusters["forward"], clusters["reverse"])
	assert.NotEqual(t, clusters["forward"], clusters["other"])
	assert.Equal(t, 2, count)
	assert.Equal(t, 1, repeated)
	assert.Equal(t, 2, largest)
}

func TestRouteDisjointSplitScoresOnlyTheFirstRideOfAnUnseenRoute(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rides := make([]rideRow, 20)
	clusters := make(map[string]int, len(rides))
	for i := range rides {
		rides[i] = rideRow{RideID: string(rune('a' + i)), Date: start.AddDate(0, 0, i)}
		clusters[rides[i].RideID] = i
	}
	// Rides at index 13 and 14 repeat the route first ridden at index 12: a
	// route-disjoint evaluation set scores that route once, on its first
	// post-cutoff appearance, not on every repeat.
	clusters[rides[13].RideID] = clusters[rides[12].RideID]
	clusters[rides[14].RideID] = clusters[rides[12].RideID]

	seen, evaluate := routeDisjointSplit(rides, clusters, 12)

	require.Equal(t, rides[:12], seen)
	seenClusters := make(map[int]bool)
	for _, ride := range seen {
		seenClusters[clusters[ride.RideID]] = true
	}
	selected := make(map[int]bool)
	for _, ride := range evaluate {
		cluster := clusters[ride.RideID]
		assert.False(t, seenClusters[cluster], "evaluation route already appeared before the cutoff")
		assert.False(t, selected[cluster], "evaluation route appeared twice")
		selected[cluster] = true
	}
	assert.Equal(t, rides[12], evaluate[0], "route first ridden at the cutoff is scored on its first appearance")
	assert.NotContains(t, evaluate, rides[13], "a repeat of an already-scored route is not scored again")
	assert.NotContains(t, evaluate, rides[14], "a repeat of an already-scored route is not scored again")
}

func TestRouteDisjointSplitReturnsNothingBelowTheMinimumGroupSize(t *testing.T) {
	rides := make([]rideRow, minGroupRides-1)
	seen, evaluate := routeDisjointSplit(rides, map[string]int{}, 0)

	assert.Nil(t, seen)
	assert.Nil(t, evaluate)
}

func TestFitRouteCoefficientsRecoversSyntheticRideTimes(t *testing.T) {
	rides := make([]rideRow, 10)
	samplesByRide := make(map[string][]sampleRow, len(rides))
	for i := range rides {
		rides[i].RideID = string(rune('a' + i))
		samplesByRide[rides[i].RideID] = rideFeatureSamples(float64(10+i), float64(50+i*i*10))
		distanceKM, ascentM := distanceAndAscent(samplesByRide[rides[i].RideID])
		rides[i].MovingSeconds = 140*distanceKM + 3.5*ascentM
	}

	secondsPerKM, secondsPerAscentM, err := fitRouteCoefficients(rides, samplesByRide)
	require.NoError(t, err)
	assert.InDelta(t, 140, secondsPerKM, 1e-6)
	assert.InDelta(t, 3.5, secondsPerAscentM, 1e-6)
}

// Physics-only and route-only, each isolated from Predict's own output, must
// average back to exactly what the hybrid model predicts, the hybrid model being
// Predict called directly. The weight is read off physicsOnlyScaleFactor rather
// than written in literally; TestPhysicsOnlyScaleFactorInvertsTheBlendWeight
// holds that factor to the real constant.
func TestHybridModelEqualsTheWeightedPhysicsAndRouteHalves(t *testing.T) {
	coefficients := testCoefficients()
	samples := rideFeatureSamples(25, 400)

	hybrid := hybridModel(&coefficients).predict(samples)
	physicsOnly := physicsOnlyModel(&coefficients).predict(samples)
	routeOnly := routeOnlyModel(coefficients.SecondsPerKM, coefficients.SecondsPerAscentM).predict(samples)

	physicsWeight := 1 / physicsOnlyScaleFactor
	assert.InDelta(t, physicsWeight*physicsOnly+(1-physicsWeight)*routeOnly, hybrid, 1e-6)
}

func TestPhysicsOnlyModelIgnoresTheRouteCoefficients(t *testing.T) {
	coefficients := testCoefficients()
	samples := rideFeatureSamples(25, 400)

	withRoute := physicsOnlyModel(&coefficients).predict(samples)
	coefficients.SecondsPerKM, coefficients.SecondsPerAscentM = 999, 999
	withDifferentRoute := physicsOnlyModel(&coefficients).predict(samples)

	assert.InDelta(t, withRoute, withDifferentRoute, 1e-9, "physics-only must not depend on the route coefficients")
}

// recalibrateConfig is the flag set every rolling-origin test runs under.
func recalibrateConfig() *runConfig {
	return &runConfig{
		etaRouteCellDegrees: defaultRouteCellDegrees, etaRouteJaccard: defaultRouteJaccardThreshold,
		etaTrainingMonths: defaultTrainingWindowMonths, recalibrate: true,
	}
}

// monthlySyntheticCorpus is syntheticCorpus spread ten days apart instead of
// one, so the rides span enough months for a rolling origin to have somewhere
// to walk.
func monthlySyntheticCorpus(rideCount int) (rides []rideRow, samplesByRide map[string][]sampleRow) {
	rides, samplesByRide = syntheticCorpus(rideCount)
	start := rides[0].Date
	for i := range rides {
		rides[i].Date = start.AddDate(0, 0, i*10)
	}

	return rides, samplesByRide
}

func syntheticCorpus(rideCount int) (rides []rideRow, samplesByRide map[string][]sampleRow) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	samplesByRide = make(map[string][]sampleRow, rideCount)
	rides = make([]rideRow, rideCount)
	for i := range rides {
		rideID := string(rune('a'+i%26)) + string(rune('0'+i/26))
		samples := rideFeatureSamples(float64(15+i%8), float64(100+(i%8)*15))
		for j := range samples {
			samples[j].Longitude += float64(i) * 0.1
		}
		samplesByRide[rideID] = samples
		distanceKM, ascentM := distanceAndAscent(samples)
		rides[i] = rideRow{
			RideID: rideID, Date: start.AddDate(0, 0, i),
			MovingSeconds: 150*distanceKM + 4*ascentM,
		}
	}

	return rides, samplesByRide
}

func TestEvaluateSplitScoresRidesAfterTheLoadedCutoffWithNoFitting(t *testing.T) {
	rides, samplesByRide := syntheticCorpus(30)
	coefficients := testCoefficients()
	cfg := &runConfig{
		etaRouteCellDegrees: defaultRouteCellDegrees, etaRouteJaccard: defaultRouteJaccardThreshold,
		etaTrainingMonths: defaultTrainingWindowMonths,
	}
	cutoff := rides[17].Date // a little over halfway through the corpus

	eval, err := evaluateSplit(rides, samplesByRide, &coefficients, cutoff, cfg)

	require.NoError(t, err)
	assert.InDelta(t, coefficients.SecondsPerKM, eval.secondsPerKM, 1e-9, "the frozen profile's own coefficients must be unchanged")
	assert.Positive(t, eval.evaluateScored)
	assert.Contains(t, eval.errorsByModel, "hybrid")
	assert.Contains(t, eval.errorsByModel, "physics-only")
	assert.Contains(t, eval.errorsByModel, "route-only")
}

// calibration_cutoff is a date, not an instant, so a ride recorded later on that
// same calendar date must still count as seen rather than as an unseen candidate
// to score. This is the bug a naive Date.After(cutoff) comparison has.
func TestEvaluateSplitTreatsTheWholeCutoffDateAsSeen(t *testing.T) {
	rides, samplesByRide := syntheticCorpus(30)
	cutoffDate := rides[17].Date
	// A same-day ride recorded that evening: after the parsed midnight
	// cutoff by time, but on the same calendar date the cutoff names.
	sameDayLate := rideRow{
		RideID: "same-day-late", Date: cutoffDate.Add(20 * time.Hour),
		MovingSeconds: rides[17].MovingSeconds,
	}
	sameDaySamples := append([]sampleRow(nil), samplesByRide[rides[17].RideID]...)
	for i := range sameDaySamples {
		sameDaySamples[i].Longitude += 5 // its own, otherwise-unseen route
	}
	samplesByRide[sameDayLate.RideID] = sameDaySamples
	withSameDayRide := append(append([]rideRow(nil), rides[:18]...), sameDayLate)
	withSameDayRide = append(withSameDayRide, rides[18:]...)

	coefficients := testCoefficients()
	cfg := &runConfig{
		etaRouteCellDegrees: defaultRouteCellDegrees, etaRouteJaccard: defaultRouteJaccardThreshold,
		etaTrainingMonths: defaultTrainingWindowMonths,
	}

	eval, err := evaluateSplit(withSameDayRide, samplesByRide, &coefficients, cutoffDate, cfg)

	require.NoError(t, err)
	// 18 original pre-cutoff rides plus the same-day-late ride: 19 seen, not
	// 18 — the naive bug would leave it out of "seen" and let it inflate the
	// evaluation count below instead.
	assert.Equal(t, 19, eval.seenCount)
	assert.Equal(t, 12, eval.evaluateScored, "the same-day ride must not inflate the evaluation set")
}

func TestRunRecalibrationRejectsACorpusTooSmallToFitFrom(t *testing.T) {
	rides, samplesByRide := monthlySyntheticCorpus(minGroupRides - 1)
	coefficients := testCoefficients()
	cfg := recalibrateConfig()
	clusters, _, _, _ := clusterRoutes(rides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	_, err := runRecalibration(rides, samplesByRide, clusters, &coefficients, cfg)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "too few hygienic rides")
}

func TestRunRecalibrationRecoversTheCorpusOwnRate(t *testing.T) {
	rides, samplesByRide := monthlySyntheticCorpus(60)
	coefficients := testCoefficients()
	coefficients.SecondsPerKM = 1.0 // deliberately wrong, so recalibration visibly moves it
	cfg := recalibrateConfig()
	clusters, _, _, _ := clusterRoutes(rides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	eval, err := runRecalibration(rides, samplesByRide, clusters, &coefficients, cfg)

	require.NoError(t, err)
	assert.InDelta(t, 150, eval.secondsPerKM, 1, "recalibration should recover the corpus's own known rate")
	assert.Positive(t, eval.folds)
	assert.Positive(t, eval.scored)
}

// The shipped fit must cover the newest rides. Withholding them — which a
// naive reading of "never train on the evaluation data" would do — deploys a
// profile deliberately blind to the months a rider most cares about.
func TestRunRecalibrationFitsThroughTheNewestRide(t *testing.T) {
	rides, samplesByRide := monthlySyntheticCorpus(60)
	coefficients := testCoefficients()
	cfg := recalibrateConfig()
	clusters, _, _, _ := clusterRoutes(rides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	eval, err := runRecalibration(rides, samplesByRide, clusters, &coefficients, cfg)

	require.NoError(t, err)
	assert.Equal(t, rides[len(rides)-1].Date, eval.calibrationCutoff)
	assert.True(t, eval.lastOrigin.Before(eval.calibrationCutoff),
		"the last fold's origin must precede the shipped fit's cutoff")
}

// Every fold must be scored on rides its own fit never saw. This is the
// property the single frozen split failed to guarantee, and the reason a
// live bias could average itself away against a stale era.
func TestRunRecalibrationNeverScoresAFoldOnItsOwnTrainingRides(t *testing.T) {
	rides, samplesByRide := monthlySyntheticCorpus(60)
	cfg := recalibrateConfig()
	clusters, _, _, _ := clusterRoutes(rides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	newest := rides[len(rides)-1].Date
	for origin := monthStart(newest.AddDate(0, -cfg.etaTrainingMonths, 0)); !origin.After(newest); origin = origin.AddDate(0, 1, 0) {
		train := trainingWindow(rides, origin, cfg.etaTrainingMonths)
		horizon := ridesBetween(rides, origin, origin.AddDate(0, rollingHorizonMonths, 0))
		for _, scoredRide := range unseenRoutesIn(horizon, train, clusters) {
			for _, trainedRide := range train {
				assert.NotEqual(t, trainedRide.RideID, scoredRide.RideID)
				assert.True(t, trainedRide.Date.Before(scoredRide.Date),
					"a fold must never train on a ride dated at or after one it scores")
			}
		}
	}
}

// The window is a bound, not a guillotine: a corpus thinner than the window
// must still produce a fit rather than an empty training set.
func TestTrainingWindowReachesPastTheWindowRatherThanStarve(t *testing.T) {
	rides, _ := monthlySyntheticCorpus(60)
	origin := rides[len(rides)-1].Date.AddDate(0, 0, 1)

	assert.Len(t, trainingWindow(rides, origin, 1), minGroupRides,
		"a one-month window holding fewer than the minimum must reach back for exactly the minimum")

	wide := trainingWindow(rides, origin, 12)
	assert.Greater(t, len(wide), minGroupRides)
	for _, ride := range wide {
		assert.False(t, ride.Date.Before(origin.AddDate(0, -12, 0)), "a full window must not reach past its own bound")
	}
}

// The copy-ready profile is what an operator pastes into ridemodel.toml, so
// #217's four validation fields must appear in it too, matching the same
// summarizeBenchmarkErrors call the human-readable line beside them uses —
// no separate computation to drift out of step.
func TestPrintCopyReadyProfileIncludesTheValidationFields(t *testing.T) {
	rides, samplesByRide := monthlySyntheticCorpus(60)
	coefficients := testCoefficients()
	cfg := recalibrateConfig()
	clusters, _, _, _ := clusterRoutes(rides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	eval, err := runRecalibration(rides, samplesByRide, clusters, &coefficients, cfg)
	require.NoError(t, err)

	metrics := summarizeBenchmarkErrors(eval.errorsByModel["hybrid"])

	var report strings.Builder
	printCopyReadyProfile(&report, &coefficients, &eval)

	printed := report.String()
	assert.Contains(t, printed, fmt.Sprintf("evaluated_rides = %d\n", metrics.rides))
	assert.Contains(t, printed, fmt.Sprintf("bias_percent = %.2f\n", metrics.bias))
	assert.Contains(t, printed, fmt.Sprintf("mae_percent = %.2f\n", metrics.mae))
	assert.Contains(t, printed, fmt.Sprintf("p90_percent = %.2f\n", metrics.p90))
}

func TestBenchmarkMetricsReportSignedBiasMAEAndP90(t *testing.T) {
	metrics := summarizeBenchmarkErrors([]float64{-10, 0, 20})

	assert.Equal(t, 3, metrics.rides)
	assert.InDelta(t, 10.0/3.0, metrics.bias, 1e-9)
	assert.InDelta(t, 10, metrics.mae, 1e-9)
	assert.InDelta(t, 18, metrics.p90, 1e-9)
}

func TestPairedMAEImprovementKeepsRidePairing(t *testing.T) {
	mean, low, high := pairedMAEImprovement([]float64{-10, 20, -30}, []float64{-5, 15, -25})

	assert.InDelta(t, 5, mean, 1e-9)
	assert.InDelta(t, 5, low, 1e-9)
	assert.InDelta(t, 5, high, 1e-9)
}

func TestBenchmarkHygieneRejectsCorruptOrIncompleteRides(t *testing.T) {
	valid := positionedSamples([][2]float64{{50.000, 8.000}, {50.002, 8.002}})
	valid[0].SpeedMPS = valid[0].IntervalDistance / valid[0].DeltaSeconds
	valid[1].SpeedMPS = valid[1].IntervalDistance / valid[1].DeltaSeconds
	ride := rideRow{RideID: "ride", MovingSeconds: 2}
	assert.Empty(t, benchmarkExclusionReason(ride, valid))

	corrupt := append([]sampleRow(nil), valid...)
	corrupt[0].SpeedMPS++
	assert.Equal(t, "inconsistent speed", benchmarkExclusionReason(ride, corrupt))

	incomplete := append([]sampleRow(nil), valid...)
	incomplete[0].HasAltitude = false
	assert.Equal(t, "incomplete geometry", benchmarkExclusionReason(ride, incomplete))
}

func TestNormalizedRideStageReturnsZeroWhenMissingGeometryExceedsTheTolerance(t *testing.T) {
	samples := rideFeatureSamples(6, 100)
	// 2 of 20 points missing (10%) is past the 5% tolerance.
	samples[1].HasPosition = false
	samples[2].HasPosition = false

	_, ok := normalizedRideStage(samples)
	assert.False(t, ok)
}

func positionedSamples(points [][2]float64) []sampleRow {
	samples := make([]sampleRow, len(points))
	for i, point := range points {
		samples[i] = sampleRow{
			Moving: true, HasPosition: true, HasAltitude: true,
			Latitude: point[0], Longitude: point[1], AltitudeM: float64(i),
			DeltaSeconds: 1, IntervalDistance: 1, SpeedMPS: 1,
		}
	}

	return samples
}

func rideFeatureSamples(distanceKM, ascentM float64) []sampleRow {
	const points = 20
	samples := make([]sampleRow, points)
	for i := range samples {
		ratio := float64(i) / (points - 1)
		samples[i] = sampleRow{
			Moving: true, HasPosition: true, HasAltitude: true,
			Latitude: 50, Longitude: 8 + ratio*distanceKM/71.5, AltitudeM: ratio * ascentM,
			DeltaSeconds: 1, IntervalDistance: distanceKM * 1000 / (points - 1),
		}
		samples[i].SpeedMPS = samples[i].IntervalDistance
	}

	return samples
}
