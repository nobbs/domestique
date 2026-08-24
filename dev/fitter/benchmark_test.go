package main

import (
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

func TestRouteDisjointFoldsContainOnlyTheFirstRideOfAnUnseenRoute(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rides := make([]rideRow, 20)
	clusters := make(map[string]int, len(rides))
	for i := range rides {
		rides[i] = rideRow{RideID: string(rune('a' + i)), Date: start.AddDate(0, 0, i)}
		clusters[rides[i].RideID] = i
	}
	clusters[rides[13].RideID] = clusters[rides[12].RideID]
	clusters[rides[14].RideID] = clusters[rides[12].RideID]

	folds := routeDisjointFolds(rides, clusters, defaultBenchmarkWarmupFraction)
	require.Len(t, folds, benchmarkFoldCount)
	for _, fold := range folds {
		seen := make(map[int]bool)
		selected := make(map[int]bool)
		for _, ride := range fold.train {
			seen[clusters[ride.RideID]] = true
		}
		for _, ride := range fold.test {
			cluster := clusters[ride.RideID]
			assert.False(t, seen[cluster], "test route already appeared in training")
			assert.False(t, selected[cluster], "test route appeared twice in one fold")
			selected[cluster] = true
		}
	}
	assert.Equal(t, []rideRow{rides[12]}, folds[0].test)
	assert.Equal(t, []rideRow{rides[15]}, folds[1].test)
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

	model, err := fitDistanceAscentModel(rides, samplesByRide)
	require.NoError(t, err)
	testSamples := rideFeatureSamples(25, 600)
	distanceKM, ascentM := distanceAndAscent(testSamples)
	assert.InDelta(t, 140*distanceKM+3.5*ascentM, model.predict(testSamples), 1e-6)
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
