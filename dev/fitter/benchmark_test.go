package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	calibrate, evaluate := routeDisjointSplit(rides, clusters, defaultBenchmarkWarmupFraction)

	require.Equal(t, rides[:12], calibrate)
	seen := make(map[int]bool)
	for _, ride := range calibrate {
		seen[clusters[ride.RideID]] = true
	}
	selected := make(map[int]bool)
	for _, ride := range evaluate {
		cluster := clusters[ride.RideID]
		assert.False(t, seen[cluster], "evaluation route already appeared in calibration")
		assert.False(t, selected[cluster], "evaluation route appeared twice")
		selected[cluster] = true
	}
	assert.Equal(t, rides[12], evaluate[0], "route first ridden at the cutoff is scored on its first appearance")
	assert.NotContains(t, evaluate, rides[13], "a repeat of an already-scored route is not scored again")
	assert.NotContains(t, evaluate, rides[14], "a repeat of an already-scored route is not scored again")
}

func TestRouteDisjointSplitReturnsNothingBelowTheMinimumGroupSize(t *testing.T) {
	rides := make([]rideRow, minGroupRides-1)
	calibrate, evaluate := routeDisjointSplit(rides, map[string]int{}, defaultBenchmarkWarmupFraction)

	assert.Nil(t, calibrate)
	assert.Nil(t, evaluate)
}

func TestDistanceAscentModelRecoversSyntheticRideTimes(t *testing.T) {
	rides := make([]rideRow, 10)
	samplesByRide := make(map[string][]sampleRow, len(rides))
	for i := range rides {
		rides[i].RideID = string(rune('a' + i))
		samplesByRide[rides[i].RideID] = rideFeatureSamples(float64(10+i), float64(50+i*i*10))
		distanceKM, ascentM := distanceAndAscent(samplesByRide[rides[i].RideID])
		rides[i].MovingSeconds = 140*distanceKM + 3.5*ascentM
	}

	model, secondsPerKM, secondsPerAscentM, err := fitDistanceAscentModel(rides, samplesByRide)
	require.NoError(t, err)
	assert.InDelta(t, 140, secondsPerKM, 1e-6)
	assert.InDelta(t, 3.5, secondsPerAscentM, 1e-6)
	testSamples := rideFeatureSamples(25, 600)
	distanceKM, ascentM := distanceAndAscent(testSamples)
	assert.InDelta(t, 140*distanceKM+3.5*ascentM, model.predict(testSamples), 1e-6)
}

// TestFitBenchmarkModelsFreezesCalibrationOnCalibrationRidesOnly is the
// property #239 exists to guarantee: the frozen candidate's route
// coefficients come only from the rides passed as train, never from rides a
// caller might later score against. Two calibration sets that share every
// ride but one must therefore fit different coefficients — if a hidden
// dependency on the evaluation rides existed, this is what would silently
// paper over it.
func TestFitBenchmarkModelsFreezesCalibrationOnCalibrationRidesOnly(t *testing.T) {
	samplesByRide := make(map[string][]sampleRow)
	rides := make([]rideRow, 12)
	for i := range rides {
		rides[i].RideID = string(rune('a' + i))
		samplesByRide[rides[i].RideID] = rideFeatureSamples(float64(10+i), float64(50+i*i*10))
		distanceKM, ascentM := distanceAndAscent(samplesByRide[rides[i].RideID])
		rides[i].MovingSeconds = 150*distanceKM + 4*ascentM
	}
	// A 13th ride within the same distance/ascent range as the rest, but
	// following a visibly different rate (200 s/km + 6 s/m instead of the
	// other twelve's 150 s/km + 4 s/m) — in-distribution enough that
	// including it shifts the fit rather than breaking it, which is the
	// property this test needs: whether fitBenchmarkModels saw it at all.
	oddOneOut := rideRow{RideID: "outlier"}
	oddSamples := rideFeatureSamples(22, 1400)
	samplesByRide[oddOneOut.RideID] = oddSamples
	oddDistanceKM, oddAscentM := distanceAndAscent(oddSamples)
	oddOneOut.MovingSeconds = 200*oddDistanceKM + 6*oddAscentM

	cfg := &runConfig{massKG: 90}
	_, secondsPerKMWithout, _, _, err := fitBenchmarkModels(rides, samplesByRide, cfg)
	require.NoError(t, err)
	_, secondsPerKMWith, _, _, err := fitBenchmarkModels(append(append([]rideRow(nil), rides...), oddOneOut), samplesByRide, cfg)
	require.NoError(t, err)

	assert.NotEqual(t, secondsPerKMWithout, secondsPerKMWith, "an outlier ride only in the second calibration set must change its fit")
}

func TestFixedPhysicsModelUsesTheAcceptedConstants(t *testing.T) {
	model := fixedPhysicsModel(90, nil)
	assert.Contains(t, model.detail, "CdA 0.45")
	assert.Contains(t, model.detail, "180 W")
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

// syntheticCorpus builds a calibratable, scoreable corpus large enough to
// clear minGroupRides on both sides of the default warm-up split, with every
// ride following the same seconds_per_km + seconds_per_ascent_m relationship
// so a route-only or hybrid fit recovers it cleanly. Positions walk east so
// every ride is its own, non-repeating route cluster.
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

func TestEvaluateSplitScoresTheFrozenHybridCandidate(t *testing.T) {
	rides, samplesByRide := syntheticCorpus(30)
	cfg := &runConfig{massKG: 90, etaWarmupFraction: defaultBenchmarkWarmupFraction}

	eval, err := evaluateSplit(rides, samplesByRide, cfg, defaultRouteCellDegrees, defaultRouteJaccardThreshold, cfg.etaWarmupFraction)

	require.NoError(t, err)
	assert.Positive(t, eval.evaluateScored)
	scalar := eval.errorsByModel["hybrid (scalar Crr)"]
	require.Len(t, scalar, eval.evaluateScored)
	// The corpus exactly follows the route-only formula, so that baseline
	// alone should recover it almost perfectly; the hybrid candidate also
	// averages in the fixed, untuned physics half and is not expected to.
	routeOnlyMetrics := summarizeBenchmarkErrors(eval.errorsByModel["route-only"])
	assert.Less(t, routeOnlyMetrics.mae, 0.5, "route-only should recover a corpus that exactly follows its own formula")
	assert.Contains(t, eval.errorsByModel, "hybrid (surface Crr)")
}

func TestEvaluateSplitReportsUnavailablePhysicsWithoutFailingTheRun(t *testing.T) {
	// A corpus with no sustained climbing never produces a valid current-physics
	// fit (fitPhysicsBenchmarkModel requires climb samples), but the hybrid
	// candidate does not depend on that fit succeeding.
	rides, samplesByRide := syntheticCorpus(30)
	cfg := &runConfig{massKG: 90, etaWarmupFraction: defaultBenchmarkWarmupFraction, climbThresholdPercent: defaultClimbThresholdPercent}

	eval, err := evaluateSplit(rides, samplesByRide, cfg, defaultRouteCellDegrees, defaultRouteJaccardThreshold, cfg.etaWarmupFraction)

	require.NoError(t, err)
	assert.NotEmpty(t, eval.physicsNote)
	assert.NotContains(t, eval.errorsByModel, "current physics")
	assert.Contains(t, eval.errorsByModel, "hybrid (scalar Crr)")
}

func TestPrintSurfaceComparisonFlagsAnUnlabelledRunAsUninformative(t *testing.T) {
	eval := splitEvaluation{errorsByModel: map[string][]float64{
		"hybrid (scalar Crr)":  {1, 2, 3},
		"hybrid (surface Crr)": {1, 2, 3},
	}}
	var report strings.Builder

	printSurfaceComparison(&report, &eval, false)

	assert.Contains(t, report.String(), "not informative")
}

func TestPrintCopyReadyProfileNamesEveryFixedConstant(t *testing.T) {
	eval := splitEvaluation{
		calibrationCutoff: time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC),
		secondsPerKM:      123.4, secondsPerAscentM: 3.21,
		errorsByModel: map[string][]float64{"hybrid (scalar Crr)": {1, -2, 3}},
	}
	cfg := &runConfig{massKG: 90}
	var report strings.Builder

	printCopyReadyProfile(&report, cfg, &eval)

	out := report.String()
	for _, want := range []string{
		"calibration_cutoff = 2025-06-01", "seconds_per_km = 123.4000", "seconds_per_ascent_m = 3.2100",
		"mass_kg = 90.0", "cda_m2 = 0.45", "power_watts = 180", "crr = 0.012",
		"asphalt = 0.012", "paving = 0.014", "compacted = 0.015", "gravel = 0.018", "ground = 0.025",
	} {
		assert.Contains(t, out, want)
	}
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
