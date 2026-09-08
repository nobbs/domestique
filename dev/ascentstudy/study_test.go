package main

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseThresholdsSplitsAndTrims(t *testing.T) {
	t.Parallel()
	got, err := parseThresholds(" 1.2, 2 ,3")
	require.NoError(t, err, "parseThresholds()")
	assert.Equal(t, []float64{1.2, 2, 3}, got)
}

func TestParseThresholdsRejectsAnUnparsableValue(t *testing.T) {
	t.Parallel()
	_, err := parseThresholds("1.2,bogus,3")
	require.ErrorContains(t, err, `parsing threshold "bogus"`)
}

func TestParseThresholdsRejectsAnEmptyList(t *testing.T) {
	t.Parallel()
	_, err := parseThresholds("  ")
	require.Error(t, err, "parseThresholds()")
}

// syntheticDeltas is one ride's altitude series expressed as adjacent steps:
// mostly a steady +1 m climb, with three deliberate pull-backs of different
// sizes so the hysteresis thresholds this study compares (1.2, 2, 3 m) do not
// all treat the series identically.
func syntheticDeltas() []float64 {
	deltas := make([]float64, 79)
	for index := range deltas {
		deltas[index] = 1
	}
	deltas[19] = -1   // smaller than every threshold
	deltas[39] = -2   // smaller than 2 and 3, not 1.2
	deltas[59] = -2.5 // smaller than 3 only

	return deltas
}

func syntheticAltitudes() []float64 {
	deltas := syntheticDeltas()
	altitudes := make([]float64, len(deltas)+1)
	altitudes[0] = 100
	for index, delta := range deltas {
		altitudes[index+1] = altitudes[index] + delta
	}

	return altitudes
}

// syntheticDistances spaces samples 25 m apart, exactly the route
// definition's own resample interval, so ProfileOf accepts the series as
// strictly increasing and every point survives Resample unchanged.
func syntheticDistances(n int) []float64 {
	distances := make([]float64, n)
	for index := range distances {
		distances[index] = float64(index) * 25
	}

	return distances
}

// seedTrackRide stores one ride whose recorded samples carry the given
// distance and altitude series, and a device-reported ascent of
// deviceAscentMetres. The position is a fixed placeholder: ActivityRideSamples
// only keeps a track point that carries one, and no real coordinate belongs
// in a synthetic fixture.
func seedTrackRide(
	t *testing.T, store *sqlite.Store, targetID string, workoutID int64,
	distanceMetres, altitudeMetres []float64, deviceAscentMetres float64,
) {
	t.Helper()
	require.Len(t, distanceMetres, len(altitudeMetres))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, store.StoreActivity(t.Context(), targetID,
		activity.Listing{ID: workoutID, TypeID: 15, LocationID: 1, Starts: start},
		activity.Summary{AscentMetres: deviceAscentMetres, Raw: []byte(`{}`)}, start), "StoreActivity()")

	records := make([]activity.Record, len(distanceMetres))
	for index := range records {
		records[index] = activity.Record{
			Time:           start.Add(time.Duration(index) * time.Second),
			DistanceMetres: distanceMetres[index], HasDistance: true,
			AltitudeMetres: altitudeMetres[index], HasAltitude: true,
			Latitude: 0, Longitude: 0, HasPosition: true,
		}
	}
	require.NoError(t, store.StoreActivityRecords(t.Context(), targetID, workoutID, activity.FIT{Records: records}),
		"StoreActivityRecords()")
}

func openStudyTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	var key [32]byte
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), key)
	require.NoError(t, err, "sqlite.Open()")
	t.Cleanup(func() { assert.NoError(t, store.Close(), "Close()") })
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	return store
}

// The study pools every candidate's relative error against the device ascent,
// skips a ride with too few samples without touching any candidate's numbers,
// and its printed report holds only aggregate counts and percentages.
func TestStudyScoresCandidatesAgainstTheDeviceAscentAndSkipsShortRides(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)
	thresholds := []float64{1.2, 2, 3}

	altitudes := syntheticAltitudes()
	distances := syntheticDistances(len(altitudes))
	const deviceAscent = 80.0
	seedTrackRide(t, store, "rider-a", 1, distances, altitudes, deviceAscent)

	// Too few track samples: counted as skipped, contributes to no candidate.
	shortAltitudes := syntheticAltitudes()[:10]
	shortDistances := syntheticDistances(len(shortAltitudes))
	seedTrackRide(t, store, "rider-a", 2, shortDistances, shortAltitudes, 50)

	result, err := study(t.Context(), store, thresholds, 60)
	require.NoError(t, err, "study()")

	assert.Equal(t, 2, result.totalRides)
	assert.Equal(t, 1, result.skippedMinSamples)
	assert.Zero(t, result.skippedZeroAscent)
	assert.Zero(t, result.skippedNonMonotonic)

	// Independently derive every candidate from the same measure package
	// functions study.go calls, over the same synthetic series, so the
	// assertions catch a wiring mistake (wrong window, wrong series, wrong
	// sign) rather than re-deriving the library's own math.
	rawAscent := measure.AscentMetres(altitudes)
	profile, ok := measure.ProfileOf(distances, altitudes)
	require.True(t, ok, "ProfileOf() on a strictly increasing odometer")
	smoothed := profile.Resample(routeIntervalMetres).MedianFiltered(routeIntervalMetres, routeWindowMetres)
	routeAscent := smoothed.AscentMetres()
	routeAltitudes := smoothed.AltitudeMetres()

	relativeErrorPercent := func(candidate float64) float64 { return (candidate/deviceAscent - 1) * 100 }

	rawSummary := result.summarize(candidateRaw)
	require.Equal(t, 1, rawSummary.rides)
	assert.InDelta(t, relativeErrorPercent(rawAscent), rawSummary.medianPercent, 1e-9)
	assert.InDelta(t, math.Abs(relativeErrorPercent(rawAscent)), rawSummary.meanAbsPercent, 1e-9)

	routeSummary := result.summarize(candidateRoute)
	require.Equal(t, 1, routeSummary.rides)
	assert.InDelta(t, relativeErrorPercent(routeAscent), routeSummary.medianPercent, 1e-9)

	for _, threshold := range thresholds {
		hystAscent := measure.AscentWithHysteresisMetres(altitudes, threshold)
		hystSummary := result.summarize(hystName(threshold))
		require.Equal(t, 1, hystSummary.rides, hystName(threshold))
		assert.InDelta(t, relativeErrorPercent(hystAscent), hystSummary.medianPercent, 1e-9, hystName(threshold))

		routeHystAscent := measure.AscentWithHysteresisMetres(routeAltitudes, threshold)
		routeHystSummary := result.summarize(routeHystName(threshold))
		require.Equal(t, 1, routeHystSummary.rides, routeHystName(threshold))
		assert.InDelta(t, relativeErrorPercent(routeHystAscent), routeHystSummary.medianPercent, 1e-9, routeHystName(threshold))
	}

	// The single ride's smallest positive step is the plain +1 m climb.
	assert.Equal(t, 1, result.quantumCounts["1"])

	golden := buildGoldenReport(t, result, thresholds, rawAscent, routeAscent, altitudes, routeAltitudes, deviceAscent)
	assert.Equal(t, golden, result.String())
	assertNoIdentifyingDigits(t, result.String())
}

// A ride whose odometer runs backwards is not a track ProfileOf accepts: it
// is counted separately and contributes nothing to the route candidates,
// while the non-route candidates still score it.
func TestStudySkipsNonMonotonicOdometerForRouteCandidatesOnly(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)

	altitudes := make([]float64, 60)
	distances := make([]float64, 60)
	for index := range altitudes {
		altitudes[index] = 100 + float64(index)
		distances[index] = float64(index) * 25
	}
	distances[30] = distances[29] - 1 // one backwards step: not a track in step order

	seedTrackRide(t, store, "rider-a", 1, distances, altitudes, 50)

	result, err := study(t.Context(), store, []float64{2}, 60)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedNonMonotonic)
	assert.Equal(t, 1, result.summarize(candidateRaw).rides, "the raw candidate needs no odometer")
	assert.Zero(t, result.summarize(candidateRoute).rides, "the route candidate needs a monotonic odometer")
}

// A ride whose device reported no climbing at all cannot be scored as a
// relative error against it, so it is skipped and counted rather than
// dividing by zero.
func TestStudySkipsAZeroDeviceAscent(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)
	altitudes := syntheticAltitudes()
	seedTrackRide(t, store, "rider-a", 1, syntheticDistances(len(altitudes)), altitudes, 0)

	result, err := study(t.Context(), store, []float64{2}, 60)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedZeroAscent)
	assert.Zero(t, result.summarize(candidateRaw).rides)
}

// buildGoldenReport renders the exact text study.go's report is expected to
// print for the single scored ride in
// TestStudyScoresCandidatesAgainstTheDeviceAscentAndSkipsShortRides, built
// independently of report.String() from the same measure-package figures.
func buildGoldenReport(
	t *testing.T, result *report, thresholds []float64,
	rawAscent, routeAscent float64, altitudes, routeAltitudes []float64, deviceAscent float64,
) string {
	t.Helper()
	line := func(name string, ascent float64) string {
		errPercent := (ascent/deviceAscent - 1) * 100
		return fmt.Sprintf("  %-14s %6d %9.1f%% %9.1f%% %9.1f%% %9.1f%%\n",
			name, 1, errPercent, errPercent, errPercent, math.Abs(errPercent))
	}

	var b string
	b += "ascent candidates vs device (relative error %, positive = candidate over-reports)\n"
	b += fmt.Sprintf("  %-14s %6s %10s %10s %10s %10s\n", "candidate", "rides", "median", "q1", "q3", "mean|err|")
	b += line(candidateRaw, rawAscent)
	b += line(candidateRoute, routeAscent)
	for _, threshold := range thresholds {
		b += line(hystName(threshold), measure.AscentWithHysteresisMetres(altitudes, threshold))
	}
	for _, threshold := range thresholds {
		b += line(routeHystName(threshold), measure.AscentWithHysteresisMetres(routeAltitudes, threshold))
	}
	b += "\n"
	b += "altimeter quantum: smallest positive altitude step per ride, bucketed\n"
	for _, label := range quantumBucketLabels() {
		b += fmt.Sprintf("  %-5s %d\n", label, result.quantumCounts[label])
	}
	b += "\n"
	b += fmt.Sprintf("rides: total=%d skipped_zero_device_ascent=%d skipped_min_samples=%d skipped_route_non_monotonic=%d\n",
		result.totalRides, result.skippedZeroAscent, result.skippedMinSamples, result.skippedNonMonotonic)

	return b
}

// assertNoIdentifyingDigits guards the tool's own stated safety promise: the
// printed report holds counts and percentages only, never a ride identifier,
// date, coordinate, slot or altitude reading.
func assertNoIdentifyingDigits(t *testing.T, printed string) {
	t.Helper()
	assert.NotContains(t, printed, "2026", "a date must never reach the report")
	assert.NotContains(t, printed, "rider-", "a target slot must never reach the report")
}

func TestParseThresholdsRejectsAThresholdThatIsNotAPositiveDistance(t *testing.T) {
	t.Parallel()
	for _, list := range []string{"0", "-2", "NaN", "Inf", "1.2,0"} {
		_, err := parseThresholds(list)
		require.Error(t, err, "parseThresholds(%q)", list)
	}
}
