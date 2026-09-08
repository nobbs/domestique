package main

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseNonNegativeSpeedsSplitsAndTrims(t *testing.T) {
	t.Parallel()
	got, err := parseNonNegativeSpeeds(" 0, 0.5 ,2")
	require.NoError(t, err, "parseNonNegativeSpeeds()")
	assert.Equal(t, []float64{0, 0.5, 2}, got)
}

func TestParseNonNegativeSpeedsRejectsANegativeOrUnparsableValue(t *testing.T) {
	t.Parallel()
	for _, list := range []string{"-1", "1,bogus", "1,-2"} {
		_, err := parseNonNegativeSpeeds(list)
		require.Error(t, err, "parseNonNegativeSpeeds(%q)", list)
	}
}

func TestParseNonNegativeSpeedsRejectsAnEmptyList(t *testing.T) {
	t.Parallel()
	_, err := parseNonNegativeSpeeds("  ")
	require.Error(t, err)
}

func TestRunRefusesAMissingDatabase(t *testing.T) {
	t.Parallel()
	require.Error(t, run("", 60, 5*time.Minute, "0,1", 10, 90))
}

func TestRunRefusesANonPositiveMinSamples(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", 0, 5*time.Minute, "0,1", 10, 90))
}

func TestRunRefusesANonPositiveBlock(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", 60, 0, "0,1", 10, 90))
}

func TestRunRefusesABadSpeedsList(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", 60, 5*time.Minute, "bogus", 10, 90))
}

func TestRunRefusesADirectionsStepOutOfRange(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", 60, 5*time.Minute, "0,1", 0, 90))
	require.Error(t, run("unused.db", 60, 5*time.Minute, "0,1", 360, 90))
}

func TestRunRefusesANonPositiveMass(t *testing.T) {
	t.Parallel()
	require.Error(t, run("unused.db", 60, 5*time.Minute, "0,1", 10, 0))
}

// A due-north bearing from one point to the next, and the first sample takes
// the second's, since it has no bearing of its own.
func TestHeadingSeriesFirstSampleTakesTheSecondsBearing(t *testing.T) {
	t.Parallel()
	track := []activity.TrackPoint{
		{Latitude: 50.0, Longitude: 8.0},
		{Latitude: 50.001, Longitude: 8.0},
		{Latitude: 50.001, Longitude: 8.001},
	}

	headings := headingSeries(track)

	require.Len(t, headings, 3)
	assert.InDelta(t, 0, headings[1], 0.5, "due north")
	assert.InDelta(t, headings[1], headings[0], 1e-9, "the first sample takes the second's bearing")
	assert.InDelta(t, 90, headings[2], 1, "due east")
}

// A single positioned point has no bearing to take at all: the series is a
// single zero.
func TestHeadingSeriesOnASinglePointIsZero(t *testing.T) {
	t.Parallel()
	headings := headingSeries([]activity.TrackPoint{{Latitude: 50, Longitude: 8}})
	assert.Equal(t, []float64{0}, headings)
}

func windTestTime(offsetSeconds int) time.Time {
	return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(offsetSeconds) * time.Second)
}

// Each estimate sample takes the nearest positioned point's heading by time,
// even where the positioned series is sparser than the estimate series.
func TestAlignHeadingsTakesTheNearestPositionByTime(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{
		{At: windTestTime(0)}, {At: windTestTime(1)}, {At: windTestTime(2)}, {At: windTestTime(10)},
	}
	positions := []activity.TrackPoint{{Time: windTestTime(0)}, {Time: windTestTime(3)}}
	headings := []float64{10, 20}

	aligned := alignHeadings(track, positions, headings)

	assert.Equal(t, []float64{10, 10, 20, 20}, aligned)
}

// A time inside a step's interval reads that step; a time before every step's
// interval falls back to the nearest one by clock distance.
func TestWeatherAtPrefersTheContainingIntervalThenTheNearest(t *testing.T) {
	t.Parallel()
	steps := []activity.WeatherStep{
		{At: windTestTime(0), Step: time.Hour, WindSpeedKMH: 10},
		{At: windTestTime(3600), Step: time.Hour, WindSpeedKMH: 20},
	}

	inside, ok := weatherAt(steps, windTestTime(1800))
	require.True(t, ok)
	assert.InDelta(t, 10, inside.WindSpeedKMH, 1e-9)

	before, ok := weatherAt(steps, windTestTime(-100))
	require.True(t, ok)
	assert.InDelta(t, 10, before.WindSpeedKMH, 1e-9, "nearest by clock distance")

	_, ok = weatherAt(nil, windTestTime(0))
	assert.False(t, ok, "no steps at all")
}

// A block under half held by a known estimate, or with no heart rate reading
// at all, is dropped; a block that clears both bars survives with its mean
// heart rate.
func TestBuildBlocksDropsAnUnderheldOrHeartRateFreeBlock(t *testing.T) {
	t.Parallel()
	const seconds = 600 // two five-minute blocks
	track := make([]measure.Sample, seconds)
	estimates := make([]measure.Estimate, seconds)
	for index := range track {
		track[index] = measure.Sample{At: windTestTime(index)}
		estimates[index] = measure.Estimate{Watts: float64(index), Known: index%2 == 0 || index < 300}
	}
	// Block 0 (index 0-299): every sample known. Block 1 (300-599): only every
	// other sample known, still >= 150 held. Heart rate is seeded for block 0
	// only, so block 1 is dropped for lacking heart rate instead.
	heartRate := make([]trainingload.Sample, 300)
	for index := range heartRate {
		heartRate[index] = trainingload.Sample{At: windTestTime(index), Value: 120}
	}

	blocks := buildBlocks(track, estimates, heartRate, 5*time.Minute)

	require.Len(t, blocks, 1)
	assert.InDelta(t, 120, blocks[0].meanHR, 1e-9)
	assert.Len(t, blocks[0].indices, 300)
}

func TestBuildBlocksIsEmptyForNoTrack(t *testing.T) {
	t.Parallel()
	assert.Nil(t, buildBlocks(nil, nil, nil, 5*time.Minute))
}

// A perfect line fits exactly, with zero residual.
func TestLinearFitAndRMSResidualFitAPerfectLineExactly(t *testing.T) {
	t.Parallel()
	heartRates := []float64{100, 120, 140, 160}
	watts := make([]float64, len(heartRates))
	for index, hr := range heartRates {
		watts[index] = 2*hr - 50
	}

	a, b := linearFit(heartRates, watts)
	assert.InDelta(t, 2, a, 1e-9)
	assert.InDelta(t, -50, b, 1e-9)
	assert.InDelta(t, 0, rmsResidual(heartRates, watts, a, b), 1e-9)
}

// hrFitRMS refuses fewer than two blocks: there is nothing to fit a line
// through.
func TestHRFitRMSNeedsAtLeastTwoBlocks(t *testing.T) {
	t.Parallel()
	_, ok := hrFitRMS(nil, []validBlock{{indices: []int{0}, meanHR: 100}})
	assert.False(t, ok)
}

// The smallest signed angle between two bearings, wrapped so 350 and 10 are
// twenty degrees apart, not three hundred and forty.
func TestAngularDifferenceWrapsAroundNorth(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 20, angularDifference(350, 10), 1e-9)
	assert.InDelta(t, 180, angularDifference(0, 180), 1e-9)
	assert.Zero(t, angularDifference(90, 90))
}

// No device calories at all is no denominator for the energy cross-check.
func TestEnergyRelErrorPercentNeedsPositiveCalories(t *testing.T) {
	t.Parallel()
	_, ok := energyRelErrorPercent(1000, 0)
	assert.False(t, ok)

	got, ok := energyRelErrorPercent(1000, 1000/4.184/0.22)
	require.True(t, ok)
	assert.InDelta(t, 0, got, 1e-6)
}

// kilojoules integrates only the known samples, over the elapsed time since
// the sample before each one.
func TestKilojoulesIntegratesOnlyKnownSamplesOverElapsedTime(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{{At: windTestTime(0)}, {At: windTestTime(1)}, {At: windTestTime(2)}}
	estimates := []measure.Estimate{{Known: false}, {Watts: 1000, Known: true}, {Watts: 0, Known: false}}

	assert.InDelta(t, 1.0, kilojoules(track, estimates), 1e-9, "1000 W for 1 s is 1 kJ")
}

// rmsVsMeasured matches each known estimate to the nearest measured power
// reading by time, and reports false with no power series at all.
func TestRMSVsMeasuredMatchesByNearestTime(t *testing.T) {
	t.Parallel()
	track := []measure.Sample{{At: windTestTime(0)}, {At: windTestTime(10)}}
	estimates := []measure.Estimate{{Watts: 100, Known: true}, {Watts: 200, Known: true}}
	power := []trainingload.Sample{{At: windTestTime(0), Value: 90}, {At: windTestTime(10), Value: 210}}

	rms, ok := rmsVsMeasured(track, estimates, power)
	require.True(t, ok)
	assert.InDelta(t, 10, rms, 1e-9)

	_, ok = rmsVsMeasured(track, estimates, nil)
	assert.False(t, ok)
}

func openWindStudyTestStore(t *testing.T) *sqlite.Store {
	t.Helper()
	var key [32]byte
	store, err := sqlite.Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), key)
	require.NoError(t, err, "sqlite.Open()")
	t.Cleanup(func() { assert.NoError(t, store.Close(), "Close()") })
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-a"), "EnsureTargetOwner()")

	return store
}

// massForTarget prefers a target's own rider profile over the flag, and falls
// back to the flag for a target with none.
func TestMassForTargetPrefersTheRiderProfileOverTheFlag(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider-b"), "EnsureTargetOwner()")
	require.NoError(t, store.SetRiderProfile(t.Context(), "rider-b",
		rider.Profile{RiderMassKG: rider.Set(70), BikeMassKG: rider.Set(10)}), "SetRiderProfile()")

	cache := map[string]float64{}
	fromFlag, err := massForTarget(t.Context(), store, "rider-a", 90, cache)
	require.NoError(t, err, "massForTarget() no profile")
	assert.InDelta(t, 90, fromFlag, 1e-9)

	fromProfile, err := massForTarget(t.Context(), store, "rider-b", 90, cache)
	require.NoError(t, err, "massForTarget() with a profile")
	assert.InDelta(t, 80, fromProfile, 1e-9)
}

// seedWindRideRecords is one ride's records: n one-second samples riding due
// north at speedMS on flat ground, with a heading and a headwind neither
// varying, so this fixture exercises the wiring rather than the fit's own
// identifiability. heartRate and power are optional per-index generators.
func seedWindRideRecords(
	n int, speedMS float64, heartRate, power func(index int) (float64, bool),
) []activity.Record {
	start := windTestTime(0)
	records := make([]activity.Record, n)
	for index := range records {
		record := activity.Record{
			Time:           start.Add(time.Duration(index) * time.Second),
			DistanceMetres: speedMS * float64(index), HasDistance: true,
			AltitudeMetres: 100, HasAltitude: true,
			Latitude: 50.0 + float64(index)*speedMS/111320.0, Longitude: 8.0, HasPosition: true,
		}
		if heartRate != nil {
			if value, ok := heartRate(index); ok {
				record.HeartRateBPM, record.HasHeartRate = value, true
			}
		}
		if power != nil {
			if value, ok := power(index); ok {
				record.PowerWatts, record.HasPower = value, true
			}
		}
		records[index] = record
	}

	return records
}

func seedWindRide(
	t *testing.T, store *sqlite.Store, targetID string, workoutID int64,
	n int, speedMS float64, heartRate, power func(index int) (float64, bool), raw string,
) {
	t.Helper()
	if raw == "" {
		raw = "{}"
	}
	require.NoError(t, store.StoreActivity(t.Context(), targetID,
		activity.Listing{ID: workoutID, TypeID: 15, LocationID: 1, Starts: windTestTime(0)},
		activity.Summary{AscentMetres: 1, Raw: []byte(raw)}, windTestTime(0)), "StoreActivity()")
	require.NoError(t, store.StoreActivityRecords(t.Context(), targetID, workoutID,
		activity.FIT{Records: seedWindRideRecords(n, speedMS, heartRate, power)}), "StoreActivityRecords()")
}

func seedWindWeather(t *testing.T, store *sqlite.Store, targetID string, workoutID int64, windSpeedKMH, windDirectionDeg float64, seconds int) {
	t.Helper()
	step := activity.WeatherStep{At: windTestTime(0), Step: time.Duration(seconds) * time.Second,
		WindSpeedKMH: windSpeedKMH, WindDirectionDegrees: windDirectionDeg}
	require.NoError(t, store.StoreActivityWeather(t.Context(), targetID, workoutID, []activity.WeatherStep{step}, windTestTime(0)),
		"StoreActivityWeather()")
}

func constantHeartRate(value float64) func(int) (float64, bool) {
	return func(int) (float64, bool) { return value, true }
}

func defaultSpeedsMS() []float64 { return []float64{0, 0.5, 1, 1.5, 2, 3, 4, 5, 6} }

// A ride below -min-samples is counted and contributes to no candidate.
func TestStudySkipsARideBelowMinSamples(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	seedWindRide(t, store, "rider-a", 1, 10, 7.5, nil, nil, "")

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.totalRides)
	assert.Equal(t, 1, result.skippedNoTrack)
	assert.Zero(t, result.metrics[metricMeanWatts].values[candidateNone])
}

// A ride with a power meter is counted separately and feeds only the meter
// block, never the main candidate tables.
func TestStudyCountsAMeterRideSeparatelyFromTheMainTables(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	seedWindRide(t, store, "rider-a", 1, 600, 7.5, constantHeartRate(140),
		func(int) (float64, bool) { return 150, true }, "")

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedHasPowerMeter)
	assert.Equal(t, 1, result.meterRides)
	assert.Empty(t, result.metrics[metricMeanWatts].values[candidateNone], "a meter ride never reaches the main tables")
	require.Contains(t, result.meterRMS, candidateNone)
	assert.Len(t, result.meterRMS[candidateNone], 1)
}

// A ride with no heart rate at all is counted, and the hrfit candidate never
// runs for it — but none still does.
func TestStudySkipsHRFitForARideWithNoHeartRate(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	seedWindRide(t, store, "rider-a", 1, 600, 7.5, nil, nil, "")

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedNoHeartRate)
	assert.Len(t, result.metrics[metricMeanWatts].values[candidateNone], 1)
	assert.Empty(t, result.metrics[metricMeanWatts].values[candidateHRFit])
}

// A ride with no recorded weather is counted, and the weather candidate never
// runs for it.
func TestStudySkipsWeatherForARideWithNoWeather(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	seedWindRide(t, store, "rider-a", 1, 600, 7.5, constantHeartRate(140), nil, "")

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedNoWeather)
	assert.Empty(t, result.metrics[metricMeanWatts].values[candidateWeather])
}

// A ride with no device calories is counted, and the energy cross-check never
// runs for it, for any candidate.
func TestStudySkipsTheEnergyCrossCheckForARideWithNoCalories(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	seedWindRide(t, store, "rider-a", 1, 600, 7.5, nil, nil, "")

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	assert.Equal(t, 1, result.skippedNoCalories)
	assert.Empty(t, result.metrics[metricEnergyError].values[candidateNone])
}

// A full ride — track, heart rate, weather and calories — feeds every table:
// none and weather always, hrfit because it has heart rate, and the energy
// cross-check because it has calories. The forecast-agreement line is
// populated too.
func TestStudyScoresAFullRideAcrossEveryCandidate(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	const n = 600
	seedWindRide(t, store, "rider-a", 1, n, 7.5, constantHeartRate(140), nil, `{"calories_accum":800}`)
	seedWindWeather(t, store, "rider-a", 1, 10, 90, n)

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	assert.Zero(t, result.skippedNoTrack)
	assert.Zero(t, result.skippedHasPowerMeter)
	assert.Zero(t, result.skippedNoHeartRate)
	assert.Zero(t, result.skippedNoWeather)
	assert.Zero(t, result.skippedNoCalories)

	for _, candidate := range candidateOrder() {
		assert.Len(t, result.metrics[metricMeanWatts].values[candidate], 1, candidate)
		assert.Len(t, result.metrics[metricEnergyError].values[candidate], 1, candidate)
	}
	require.Len(t, result.speedDiffMS, 1)
	require.Len(t, result.angleDiffDeg, 1)
	assertReportHoldsNoIdentifyingDetail(t, result.String())
}

// The printed report holds only aggregate numbers: no date, no ride or
// target identifier, ever reaches it.
func assertReportHoldsNoIdentifyingDetail(t *testing.T, printed string) {
	t.Helper()
	assert.NotContains(t, printed, "2026", "a date must never reach the report")
	assert.NotContains(t, printed, "rider-", "a target slot must never reach the report")
}

// A golden rendering of the simplest qualifying ride: track and heart rate
// only, no weather and no calories, so only "none" and "hrfit" ever appear.
func TestStudyGoldenReportForARideWithNoWeatherAndNoCalories(t *testing.T) {
	t.Parallel()
	store := openWindStudyTestStore(t)
	const n = 600
	seedWindRide(t, store, "rider-a", 1, n, 7.5, constantHeartRate(140), nil, "")

	result, err := study(t.Context(), store, 60, 5*time.Minute, defaultSpeedsMS(), 10, 82)
	require.NoError(t, err, "study()")

	golden := newReport()
	golden.totalRides = 1
	golden.skippedNoWeather = 1

	noneEstimates, noneQuality, ok := measure.EstimateSeries(seedTrackFor(n, 7.5), 82)
	require.True(t, ok)
	blocks := buildBlocks(seedTrackFor(n, 7.5), noneEstimates, constantHeartRateSamples(n, 140), 5*time.Minute)
	golden.recordCandidate(candidateNone, seedTrackFor(n, 7.5), noneEstimates, noneQuality, blocks, 0, false)

	assert.Equal(t, golden.metrics[metricMeanWatts].values[candidateNone], result.metrics[metricMeanWatts].values[candidateNone])
	assert.Equal(t, golden.metrics[metricAutocorr].values[candidateNone], result.metrics[metricAutocorr].values[candidateNone])
	assert.Equal(t, golden.metrics[metricHRFitRMS].values[candidateNone], result.metrics[metricHRFitRMS].values[candidateNone])
	assert.NotEmpty(t, result.metrics[metricMeanWatts].values[candidateHRFit], "heart rate is present, so hrfit ran")
	assertReportHoldsNoIdentifyingDetail(t, result.String())
}

// seedTrackFor is the same flat, due-north track seedWindRideRecords stores,
// expressed as the measure.Sample series EstimateSeries takes directly — used
// to derive the golden report's own expected numbers from the same library
// calls study.go makes, rather than re-deriving its math by hand.
func seedTrackFor(n int, speedMS float64) []measure.Sample {
	samples := make([]measure.Sample, n)
	for index := range samples {
		samples[index] = measure.Sample{
			At: windTestTime(index), DistanceMetres: speedMS * float64(index), AltitudeMetres: 100,
		}
	}

	return samples
}

func constantHeartRateSamples(n int, value float64) []trainingload.Sample {
	samples := make([]trainingload.Sample, n)
	for index := range samples {
		samples[index] = trainingload.Sample{At: windTestTime(index), Value: value}
	}

	return samples
}

// The synthetic recovery test from the handover's own §6: a ride with a known
// heading loop and heart rate generated as linear in the true wind's power
// plus small deterministic noise recovers that wind within one grid step in
// both speed and direction.
func TestFitWindFromHeartRateRecoversAKnownWindWithinOneGridStep(t *testing.T) {
	t.Parallel()
	const (
		n                = 1200 // four five-minute quarters
		mass             = 80.0
		trueSpeedMS      = 3.0
		trueDirectionDeg = 90.0
		hrPerWatt        = 0.4
		hrBaseline       = 90.0
	)
	// A square loop: due north, then east, then south, then west, one quarter
	// each — heading spread across all four quadrants is what makes a wind
	// vector identifiable at all (handover §6). Each leg is also ridden at its
	// own pace, so a calm candidate's power is not already a constant that
	// would fit any heart-rate series exactly regardless of wind.
	quarterSpeedsMS := []float64{6.0, 8.0, 7.0, 9.0}
	quarterHeadings := []float64{0, 90, 180, 270}
	track := make([]measure.Sample, n)
	headings := make([]float64, n)
	distance := 0.0
	for index := range track {
		quarter := index / (n / 4)
		if index > 0 {
			distance += quarterSpeedsMS[quarter]
		}
		track[index] = measure.Sample{At: windTestTime(index), DistanceMetres: distance, AltitudeMetres: 100}
		headings[index] = quarterHeadings[quarter]
	}
	trueHeadwind := make([]float64, n)
	for index, heading := range headings {
		trueHeadwind[index] = trueSpeedMS * math.Cos((trueDirectionDeg-heading)*math.Pi/180)
	}
	trueEstimates, _, ok := measure.EstimateSeriesWithWind(track, mass, trueHeadwind)
	require.True(t, ok)

	noiseCycle := []float64{0, 1, -1, 0.5, -0.5}
	heartRate := make([]trainingload.Sample, n)
	for index := range heartRate {
		heartRate[index] = trainingload.Sample{
			At:    windTestTime(index),
			Value: hrPerWatt*trueEstimates[index].Watts + hrBaseline + noiseCycle[index%len(noiseCycle)],
		}
	}

	blocks := buildBlocks(track, trueEstimates, heartRate, 5*time.Minute)
	require.GreaterOrEqual(t, len(blocks), 2)

	speedsMS := defaultSpeedsMS()
	const directionStepDeg = 10.0
	bestSpeed, bestDirection, _, fitOK := fitWindFromHeartRate(track, headings, mass, blocks, speedsMS, directionStepDeg)
	require.True(t, fitOK)

	assert.InDelta(t, trueSpeedMS, bestSpeed, 1.0, "within one grid step in speed")
	assert.LessOrEqual(t, angularDifference(bestDirection, trueDirectionDeg), directionStepDeg,
		"within one grid step in direction")
}

func TestReportStringMentionsEveryMetric(t *testing.T) {
	t.Parallel()
	r := newReport()
	printed := r.String()
	for _, name := range metricOrder() {
		assert.Contains(t, printed, name)
	}
	assert.Contains(t, printed, "rides with a meter: 0")
}
