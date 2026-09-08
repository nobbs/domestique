package main

import (
	"context"
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

func TestParseThresholdsRejectsAThresholdThatIsNotAPositiveDistance(t *testing.T) {
	t.Parallel()
	for _, list := range []string{"0", "-2", "NaN", "Inf", "1.2,0"} {
		_, err := parseThresholds(list)
		require.Error(t, err, "parseThresholds(%q)", list)
	}
}

func TestParseThresholdsRejectsAThresholdListedTwice(t *testing.T) {
	t.Parallel()
	_, err := parseThresholds("2,3,2")
	require.Error(t, err)
}

func TestParseGridsSplitsAndTrims(t *testing.T) {
	t.Parallel()
	got, err := parseGrids(" 10, 20 ,50")
	require.NoError(t, err, "parseGrids()")
	assert.Equal(t, []float64{10, 20, 50}, got)
}

func TestParseGridsRejectsAGridThatIsNotAPositiveDistance(t *testing.T) {
	t.Parallel()
	for _, list := range []string{"0", "-10", "NaN", "10,-1"} {
		_, err := parseGrids(list)
		require.Error(t, err, "parseGrids(%q)", list)
	}
}

func TestParseGridsRejectsAGridListedTwice(t *testing.T) {
	t.Parallel()
	_, err := parseGrids("10,20,10")
	require.Error(t, err)
}

func TestValidateStillSpeedRejectsANonPositiveOrNonFiniteSpeed(t *testing.T) {
	t.Parallel()
	for _, value := range []float64{0, -1, math.NaN(), math.Inf(1)} {
		require.Error(t, validateStillSpeed(value))
	}
	require.NoError(t, validateStillSpeed(1.0))
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
	seedTrackRideWithSummary(t, store, targetID, workoutID, distanceMetres, altitudeMetres, deviceAscentMetres, 0, 0)
}

// seedTrackRideWithSummary is seedTrackRide with the ride's own summary
// distance and moving time also set, for a fixture that needs to land in a
// particular speed bucket or contribute a drift figure — both read from the
// ride summary, not the FIT samples.
func seedTrackRideWithSummary(
	t *testing.T, store *sqlite.Store, targetID string, workoutID int64,
	distanceMetres, altitudeMetres []float64,
	deviceAscentMetres, summaryDistanceMetres, summaryMovingSeconds float64,
) {
	t.Helper()
	require.Len(t, distanceMetres, len(altitudeMetres))
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	require.NoError(t, store.StoreActivity(t.Context(), targetID,
		activity.Listing{ID: workoutID, TypeID: 15, LocationID: 1, Starts: start},
		activity.Summary{
			AscentMetres: deviceAscentMetres, Raw: []byte(`{}`),
			DistanceMetres: summaryDistanceMetres, MovingSeconds: summaryMovingSeconds,
		}, start), "StoreActivity()")

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

// seedWeather records one hour of weather against an already-stored ride, so
// it counts toward the weather split's wet (precipitationMillimetres > 0) or
// dry group instead of "no weather".
func seedWeather(t *testing.T, store *sqlite.Store, targetID string, workoutID int64, precipitationMillimetres float64) {
	t.Helper()
	step := activity.WeatherStep{
		At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Step: time.Hour,
		PrecipitationMillimetres: precipitationMillimetres,
	}
	require.NoError(t, store.StoreActivityWeather(t.Context(), targetID, workoutID, []activity.WeatherStep{step}, time.Now()),
		"StoreActivityWeather()")
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

// defaultGrids and defaultThresholds mirror ascentstudy's own flag defaults,
// so a test run stays comparable to a real one over the same flags.
func defaultGrids() []float64      { return []float64{10, 20, 50} }
func defaultThresholds() []float64 { return []float64{1.2, 2, 3} }

// The study pools every candidate's relative error against the device ascent,
// skips a ride with too few samples without touching any candidate's numbers,
// and its printed report holds only aggregate counts and percentages.
func TestStudyScoresCandidatesAgainstTheDeviceAscentAndSkipsShortRides(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)
	thresholds := defaultThresholds()
	grids := defaultGrids()

	altitudes := syntheticAltitudes()
	distances := syntheticDistances(len(altitudes))
	const deviceAscent = 80.0
	seedTrackRide(t, store, "rider-a", 1, distances, altitudes, deviceAscent)

	// Too few track samples: counted as skipped, contributes to no candidate.
	shortAltitudes := syntheticAltitudes()[:10]
	shortDistances := syntheticDistances(len(shortAltitudes))
	seedTrackRide(t, store, "rider-a", 2, shortDistances, shortAltitudes, 50)

	result, err := study(t.Context(), store, thresholds, grids, 1.0, 60, true)
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

		// Every sample moves 25 m per 1 s step, far above the default 1 m/s
		// still-speed: nothing is filtered, so this matches hyst<T> exactly.
		stillSummary := result.summarize(stillHystName(threshold))
		require.Equal(t, 1, stillSummary.rides, stillHystName(threshold))
		assert.InDelta(t, relativeErrorPercent(hystAscent), stillSummary.medianPercent, 1e-9, stillHystName(threshold))

		for _, grid := range grids {
			gridAscent := measure.AscentWithHysteresisMetres(profile.Resample(grid).AltitudeMetres(), threshold)
			gridSummary := result.summarize(gridHystName(grid, threshold))
			require.Equal(t, 1, gridSummary.rides, gridHystName(grid, threshold))
			assert.InDelta(t, relativeErrorPercent(gridAscent), gridSummary.medianPercent, 1e-9, gridHystName(grid, threshold))
		}
	}

	// The single ride's smallest positive step is the plain +1 m climb.
	assert.Equal(t, 1, result.quantumCounts["1"])

	// Neither ride carries a weather summary or a moving-time summary: the
	// scored ride lands in "no weather" and no speed bucket at all.
	golden := buildGoldenReport(t, thresholds, grids, altitudes, routeAltitudes, deviceAscent)
	assert.Equal(t, golden, result.String())
	assertNoIdentifyingDigits(t, result.String())
}

// A ride whose odometer runs backwards is not a track ProfileOf accepts: it
// is counted separately and contributes nothing to the route or grid
// candidates, while the non-route candidates still score it.
func TestStudySkipsNonMonotonicOdometerForRouteAndGridCandidatesOnly(t *testing.T) {
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

	result, err := study(t.Context(), store, []float64{2}, []float64{20}, 1.0, 60, false)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedNonMonotonic)
	assert.Equal(t, 1, result.summarize(candidateRaw).rides, "the raw candidate needs no odometer")
	assert.Equal(t, 1, result.summarize(stillHystName(2)).rides, "the stationary filter needs no odometer")
	assert.Zero(t, result.summarize(candidateRoute).rides, "the route candidate needs a monotonic odometer")
	assert.Zero(t, result.summarize(gridHystName(20, 2)).rides, "the grid candidate needs a monotonic odometer")
}

// A ride whose device reported no climbing at all cannot be scored as a
// relative error against it, so it is skipped and counted rather than
// dividing by zero.
func TestStudySkipsAZeroDeviceAscent(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)
	altitudes := syntheticAltitudes()
	seedTrackRide(t, store, "rider-a", 1, syntheticDistances(len(altitudes)), altitudes, 0)

	result, err := study(t.Context(), store, []float64{2}, []float64{20}, 1.0, 60, false)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedZeroAscent)
	assert.Zero(t, result.summarize(candidateRaw).rides)
}

// The stationary filter drops a sample whose speed since the previous one
// falls below the still-speed floor — including a spurious altitude reading
// recorded while parked — and keeps every sample that was actually moving.
func TestStillFilteredAltitudesDropsStandstillSamplesAndKeepsTheRest(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	track := []measure.Sample{
		{At: start, DistanceMetres: 0, AltitudeMetres: 100},
		{At: start.Add(1 * time.Second), DistanceMetres: 5, AltitudeMetres: 101},  // 5 m/s: moving
		{At: start.Add(2 * time.Second), DistanceMetres: 5, AltitudeMetres: 102},  // 0 m/s: standstill noise
		{At: start.Add(3 * time.Second), DistanceMetres: 5, AltitudeMetres: 99},   // 0 m/s: standstill noise
		{At: start.Add(4 * time.Second), DistanceMetres: 10, AltitudeMetres: 105}, // 5 m/s: moving
	}

	got := stillFilteredAltitudes(track, 1.0)

	assert.Equal(t, []float64{100, 101, 105}, got)
}

// A step with non-positive elapsed time — a repeated or reordered
// timestamp — is dropped outright: speed cannot be judged for it.
func TestStillFilteredAltitudesDropsANonPositiveTimeStep(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	track := []measure.Sample{
		{At: start, DistanceMetres: 0, AltitudeMetres: 100},
		{At: start, DistanceMetres: 10, AltitudeMetres: 110}, // dt=0
	}

	got := stillFilteredAltitudes(track, 1.0)

	assert.Equal(t, []float64{100}, got)
}

// The distance-grid candidate resamples the odometer to a fixed spacing
// before hysteresis, so a dip narrower than the grid never reaches the
// threshold check at all — a coarser grid than the dip's own width smooths
// it away before hysteresis gets a look, exactly as a hand trace of the
// fixture predicts.
func TestStudyGridCandidateResamplesTheOdometerBeforeHysteresis(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)

	// A short dip and recovery at 10 m sits between two grid points 20 m
	// apart, so a 20 m grid never samples it.
	distances := []float64{0, 10, 20, 30, 40, 50}
	altitudes := []float64{100, 98, 104, 100, 108, 110}
	const deviceAscent = 16.0 // = grid10+hyst3's own hand-traced figure
	seedTrackRide(t, store, "rider-a", 1, distances, altitudes, deviceAscent)

	result, err := study(t.Context(), store, []float64{3}, []float64{10, 20}, 1.0, len(altitudes), false)
	require.NoError(t, err, "study()")

	// grid10 keeps every original sample: hysteresis at 3 m closes the climb
	// at the dip (104 -> 100 is a 4 m pull-back) before reopening it, for
	// (104-98) + (110-100) = 6 + 10 = 16 m.
	fullResolution := result.summarize(gridHystName(10, 3))
	require.Equal(t, 1, fullResolution.rides)
	assert.InDelta(t, 0, fullResolution.medianPercent, 1e-9, "16 m against a device ascent of 16 m")

	// grid20 samples only 0, 20, 40 and the ride's own close at 50: a smooth
	// 100 -> 104 -> 108 -> 110 climb of 10 m, no reversal to trip 3 m of
	// hysteresis.
	coarse := result.summarize(gridHystName(20, 3))
	require.Equal(t, 1, coarse.rides)
	assert.InDelta(t, (10.0/deviceAscent-1)*100, coarse.medianPercent, 1e-9)
}

// A ride's weather summary (or its absence) sorts it into the wet, dry, or
// "no weather" split, and its own mean moving speed (distance over moving
// time) sorts it into a speed bucket — independently of one another.
func TestStudySplitsRidesByWeatherAndByMeanMovingSpeed(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)
	altitudes := syntheticAltitudes()
	distances := syntheticDistances(len(altitudes))

	seedTrackRideWithSummary(t, store, "rider-a", 1, distances, altitudes, 80, 1000, 250) // 4 m/s: < 5
	seedWeather(t, store, "rider-a", 1, 2.5)                                              // wet

	seedTrackRideWithSummary(t, store, "rider-a", 2, distances, altitudes, 80, 1200, 200) // 6 m/s: 5-7
	seedWeather(t, store, "rider-a", 2, 0)                                                // dry

	seedTrackRideWithSummary(t, store, "rider-a", 3, distances, altitudes, 80, 1600, 200) // 8 m/s: >= 7
	// no weather seeded for ride 3

	result, err := study(t.Context(), store, []float64{2}, []float64{20}, 1.0, 60, true)
	require.NoError(t, err, "study()")

	ridesIn := func(splits []namedReport, label string) int {
		t.Helper()
		for _, entry := range splits {
			if entry.label == label {
				return entry.report.summarize(candidateRaw).rides
			}
		}
		t.Fatalf("split %q not found", label)

		return 0
	}

	assert.Equal(t, 1, ridesIn(result.weatherSplits, "wet"))
	assert.Equal(t, 1, ridesIn(result.weatherSplits, "dry"))
	assert.Equal(t, 1, ridesIn(result.weatherSplits, "no weather"))
	assert.Equal(t, 1, ridesIn(result.speedSplits, "<5 m/s"))
	assert.Equal(t, 1, ridesIn(result.speedSplits, "5-7 m/s"))
	assert.Equal(t, 1, ridesIn(result.speedSplits, ">=7 m/s"))
}

// -splits false drops the split block from the report entirely.
func TestStudyOmitsSplitBlockWhenDisabled(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)
	altitudes := syntheticAltitudes()
	seedTrackRide(t, store, "rider-a", 1, syntheticDistances(len(altitudes)), altitudes, 80)

	result, err := study(t.Context(), store, []float64{2}, []float64{20}, 1.0, 60, false)
	require.NoError(t, err, "study()")

	assert.NotContains(t, result.String(), "split:")
}

// (ascent - descent) - (last - first) on the same raw altitude series is an
// identity, true of any series by construction of AscentMetres and
// DescentMetres: it is a wiring check on study.go's own drift formula, not
// something a real barometer reading can make non-zero.
func TestStudyDriftIsTheHeightARideThatReturnsToItsStartGainedOrLost(t *testing.T) {
	t.Parallel()
	store := openStudyTestStore(t)

	// Every synthetic record sits at one coordinate, so each ride returns to
	// its start by position; only the altitude decides the drift.
	loopDeltas := make([]float64, 80)
	for index := range loopDeltas[:40] {
		loopDeltas[index] = 1
	}
	for index := 40; index < 80; index++ {
		loopDeltas[index] = -1
	}
	loopAltitudes := make([]float64, len(loopDeltas)+1)
	loopAltitudes[0] = 100
	for index, delta := range loopDeltas {
		loopAltitudes[index+1] = loopAltitudes[index] + delta
	}
	seedTrackRideWithSummary(t, store, "rider-a", 1,
		syntheticDistances(len(loopAltitudes)), loopAltitudes, 40, 2000, 3600)

	// A ride that ends 80 m higher than it began, over one moving hour.
	openAltitudes := syntheticAltitudes()
	seedTrackRideWithSummary(t, store, "rider-a", 2,
		syntheticDistances(len(openAltitudes)), openAltitudes, 80, 2000, 3600)

	result, err := study(t.Context(), store, []float64{2}, []float64{20}, 1.0, 60, false)
	require.NoError(t, err, "study()")

	require.Len(t, result.driftPerHourMetres, 2)
	assert.InDelta(t, 0, result.driftPerHourMetres[0], 1e-6, "the loop came back to its own height")
	assert.InDelta(t, openAltitudes[len(openAltitudes)-1]-openAltitudes[0], result.driftPerHourMetres[1], 1e-6,
		"the climb's whole rise reads as drift over its one hour")
}
func buildGoldenReport(
	t *testing.T, thresholds, grids, altitudes, routeAltitudes []float64, deviceAscent float64,
) string {
	t.Helper()
	profile, ok := measure.ProfileOf(syntheticDistances(len(altitudes)), altitudes)
	require.True(t, ok, "ProfileOf()")

	expected := newReportWithOrder(fullCandidateOrder(thresholds, grids))
	expected.totalRides = 2
	expected.skippedMinSamples = 1
	expected.quantumCounts["1"] = 1

	expected.record(candidateRaw, measure.AscentMetres(altitudes), deviceAscent)
	expected.record(candidateRoute, measure.AscentMetres(routeAltitudes), deviceAscent)
	for _, threshold := range thresholds {
		expected.record(hystName(threshold), measure.AscentWithHysteresisMetres(altitudes, threshold), deviceAscent)
	}
	for _, threshold := range thresholds {
		expected.record(routeHystName(threshold), measure.AscentWithHysteresisMetres(routeAltitudes, threshold), deviceAscent)
	}
	for _, grid := range grids {
		gridAltitudes := profile.Resample(grid).AltitudeMetres()
		for _, threshold := range thresholds {
			expected.record(gridHystName(grid, threshold), measure.AscentWithHysteresisMetres(gridAltitudes, threshold), deviceAscent)
		}
	}
	for _, threshold := range thresholds {
		// Every sample moves 25 m per 1 s step: the default 1 m/s still-speed
		// filters nothing, so this matches hyst<T> exactly.
		expected.record(stillHystName(threshold), measure.AscentWithHysteresisMetres(altitudes, threshold), deviceAscent)
	}

	expected.splitsEnabled = true
	splitOrder := splitCandidateOrder(expected.order)
	for _, label := range weatherSplitLabels() {
		sub := newReportWithOrder(splitOrder)
		if label == "no weather" {
			for _, name := range splitOrder {
				sub.errorsPercent[name] = append([]float64(nil), expected.errorsPercent[name]...)
			}
		}
		expected.weatherSplits = append(expected.weatherSplits, namedReport{label: label, report: sub})
	}
	for _, label := range speedSplitLabels() {
		expected.speedSplits = append(expected.speedSplits, namedReport{label: label, report: newReportWithOrder(splitOrder)})
	}

	return expected.String()
}

// assertNoIdentifyingDigits guards the tool's own stated safety promise: the
// printed report holds counts and percentages only, never a ride identifier,
// date, coordinate, slot or altitude reading.
func assertNoIdentifyingDigits(t *testing.T, printed string) {
	t.Helper()
	assert.NotContains(t, printed, "2026", "a date must never reach the report")
	assert.NotContains(t, printed, "rider-", "a target slot must never reach the report")
}

func TestRunRefusesAMinSamplesThatIsNotPositive(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", "2", "10,20", 1.0, 0, true))
}

func TestRunRefusesABadGridList(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", "2", "bogus", 1.0, 60, true))
}

func TestRunRefusesABadStillSpeed(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", "2", "10,20", 0, 60, true))
}
