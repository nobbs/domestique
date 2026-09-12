package trainingload_test

import (
	"math"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rideStart() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

// steady records one value once a second for the given number of seconds.
func steady(seconds int, value float64) []trainingload.Sample {
	samples := make([]trainingload.Sample, seconds)
	for index := range samples {
		samples[index] = trainingload.Sample{At: rideStart().Add(time.Duration(index) * time.Second), Value: value}
	}

	return samples
}

// blocks records one value then another in runs of the given length. A run
// shorter than the rolling window is exactly what normalized power is built to
// smooth away, so a surge worth more than its average has to outlast one.
func blocks(seconds, run int, low, high float64) []trainingload.Sample {
	samples := make([]trainingload.Sample, seconds)
	for index := range samples {
		value := low
		if (index/run)%2 == 1 {
			value = high
		}
		samples[index] = trainingload.Sample{At: rideStart().Add(time.Duration(index) * time.Second), Value: value}
	}

	return samples
}

func TestBoundsPreferTheThresholdOverTheMaximum(t *testing.T) {
	t.Parallel()
	fromThreshold, ok := trainingload.BoundsFrom(170, 190)
	require.True(t, ok)
	assert.InDelta(t, 144.5, fromThreshold[0], 0.01, "85% of the threshold, not a share of the maximum")

	fromMaximum, ok := trainingload.BoundsFrom(0, 190)
	require.True(t, ok)
	assert.InDelta(t, 114.0, fromMaximum[0], 0.01, "60% of the maximum")

	_, ok = trainingload.BoundsFrom(0, 0)
	assert.False(t, ok, "a profile with neither rate cuts no zones")
}

// The acceptance criterion: the zone times account for the ride, to within the
// one sample at the end that stands for nothing.
func TestTimeInZonesAccountsForTheWholeRide(t *testing.T) {
	t.Parallel()
	bounds, ok := trainingload.BoundsFrom(0, 200)
	require.True(t, ok)

	zones := trainingload.TimeInZones(steady(3600, 150), bounds)
	assert.InDelta(t, 3599.0, zones.Total(), 1.0, "the whole ride bar its last sample")
	assert.InDelta(t, 3599.0, zones[2], 1.0, "150 of 200 is 75%, the third zone")
	assert.Zero(t, zones[0], "and nothing anywhere else")
}

func TestTimeInZonesSplitsAcrossTheBounds(t *testing.T) {
	t.Parallel()
	bounds, ok := trainingload.BoundsFrom(0, 200)
	require.True(t, ok)

	// 110 is below 60% and 190 is above 90%: the easiest zone and the hardest.
	zones := trainingload.TimeInZones(blocks(600, 1, 110, 190), bounds)
	assert.InDelta(t, 300.0, zones[0], 1.0, "half the ride easy")
	assert.InDelta(t, 299.0, zones[4], 1.0, "and half hard")
}

// A recorder that paused must not book the whole pause to the zone it stopped
// in, which would put a coffee stop in the rider's training record.
func TestTimeInZonesWillNotBookARecordingGap(t *testing.T) {
	t.Parallel()
	bounds, ok := trainingload.BoundsFrom(0, 200)
	require.True(t, ok)
	samples := []trainingload.Sample{
		{At: rideStart(), Value: 150},
		{At: rideStart().Add(time.Second), Value: 150},
		{At: rideStart().Add(time.Hour), Value: 150},
	}

	assert.InDelta(t, 1.0, trainingload.TimeInZones(samples, bounds).Total(), 0.01,
		"one second, not an hour and one")
}

func TestSeriesCoverageIsFullWhenTheSeriesHeldForTheWholeMovingTime(t *testing.T) {
	t.Parallel()
	share, ok := trainingload.SeriesCoverage(steady(3600, 140), 3599)
	require.True(t, ok)
	assert.InDelta(t, 1, share, 1e-9)
}

// A strap live through a stop the odometer does not count as moving still
// covers the moving time in full — coverage is never claimed above it.
func TestSeriesCoverageIsCappedAtOneWhenHeldExceedsMovingTime(t *testing.T) {
	t.Parallel()
	share, ok := trainingload.SeriesCoverage(steady(3600, 140), 1800)
	require.True(t, ok)
	assert.InDelta(t, 1.0, share, 1e-9)
}

func TestSeriesCoverageIsPartialWhenTheSeriesDroppedOutPartway(t *testing.T) {
	t.Parallel()
	// 600 samples a second apart hold for 599 seconds: the last stands for nothing.
	share, ok := trainingload.SeriesCoverage(steady(600, 140), 3600)
	require.True(t, ok)
	assert.InDelta(t, 599.0/3600, share, 1e-9)
}

// A meter's nought is a coast, not a dropout: it counts as a reading.
func TestSeriesCoverageCountsAMetersNought(t *testing.T) {
	t.Parallel()
	share, ok := trainingload.SeriesCoverage(steady(3601, 0), 3600)
	require.True(t, ok)
	assert.InDelta(t, 1, share, 1e-9)
}

func TestSeriesCoverageIsUnknownForAnEmptySeries(t *testing.T) {
	t.Parallel()
	_, ok := trainingload.SeriesCoverage(nil, 3600)
	assert.False(t, ok)
}

func TestSeriesCoverageIsUnknownWithoutAMovingTimeToShareOf(t *testing.T) {
	t.Parallel()
	_, ok := trainingload.SeriesCoverage(steady(600, 140), 0)
	assert.False(t, ok)
}

func TestTRIMPNeedsAReserveToMeasureAgainst(t *testing.T) {
	t.Parallel()
	_, ok := trainingload.TRIMP(steady(600, 150), 0, 50)
	assert.False(t, ok, "no maximum, no reserve")
	_, ok = trainingload.TRIMP(steady(600, 150), 190, 190)
	assert.False(t, ok, "a reserve of nothing is not a reserve")
}

// Banister's own formula on a fixed input: ten minutes at half the reserve.
func TestTRIMPMatchesBanistersFormula(t *testing.T) {
	t.Parallel()
	trimp, ok := trainingload.TRIMP(steady(601, 120), 190, 50)
	require.True(t, ok)

	fraction := (120.0 - 50) / (190 - 50)
	want := 10 * fraction * 0.64 * math.Exp(1.92*fraction)
	assert.InDelta(t, want, trimp, 0.01)
}

// A maximum entered too low would otherwise put the exponential somewhere no
// ride belongs.
func TestTRIMPClampsARateAboveTheEnteredMaximum(t *testing.T) {
	t.Parallel()
	atMaximum, ok := trainingload.TRIMP(steady(601, 190), 190, 50)
	require.True(t, ok)
	above, ok := trainingload.TRIMP(steady(601, 230), 190, 50)
	require.True(t, ok)

	assert.InDelta(t, atMaximum, above, 0.01, "a rate over the maximum counts as the maximum")
}

// An hour held at the threshold is a hundred, which is what makes the number
// mean the same thing as a power stress score.
func TestHeartRateTSSScoresAnHourAtThresholdAsAHundred(t *testing.T) {
	t.Parallel()
	score, ok := trainingload.HeartRateTSS(steady(3601, 170), 170, 50)
	require.True(t, ok)
	assert.InDelta(t, 100.0, score, 0.1)

	_, ok = trainingload.HeartRateTSS(steady(3601, 170), 0, 50)
	assert.False(t, ok, "without a threshold there is no intensity to square")

	_, ok = trainingload.HeartRateTSS(steady(3601, 170), 170, 0)
	assert.False(t, ok, "nor without a resting rate to measure the reserve from")
}

// The regression: heart rate does not fall to zero as power does, so scoring a
// ride on the bare ratio of its mean to the threshold credits the rider for
// simply being alive. An easy ride is an easy ride.
func TestHeartRateTSSDoesNotScoreAnEasyRideAsThreshold(t *testing.T) {
	t.Parallel()
	// Two hours at 120 against a threshold of 148 and a resting rate of 45: the
	// bare ratio calls that 0.81 of threshold, the reserve calls it 0.72.
	easy, ok := trainingload.HeartRateTSS(steady(7201, 120), 148, 45)
	require.True(t, ok)
	bareRatio := 2 * (120.0 / 148.0) * (120.0 / 148.0) * 100

	assert.Less(t, easy, bareRatio, "the reserve scores it below the bare ratio")
	assert.InDelta(t, 106.0, easy, 0.5)
}

// The acceptance criterion: normalized power is never below the average, and
// rises above it exactly where the ride surged.
func TestNormalizedPowerIsNeverBelowTheAverage(t *testing.T) {
	t.Parallel()
	level, ok := trainingload.PowerLoad(steady(3601, 200), 250)
	require.True(t, ok)
	assert.InDelta(t, 200.0, level.NormalizedWatts, 1.0, "a level ride normalizes to its own power")

	// One-second alternation averages away inside the window, which is the point
	// of it; two-minute blocks of the same two powers do not.
	flickering, ok := trainingload.PowerLoad(blocks(3601, 1, 100, 300), 250)
	require.True(t, ok)
	assert.InDelta(t, 200.0, flickering.NormalizedWatts, 1.0, "a flicker is smoothed, not punished")

	surging, ok := trainingload.PowerLoad(blocks(3601, 120, 100, 300), 250)
	require.True(t, ok)
	assert.Greater(t, surging.NormalizedWatts, 210.0,
		"the same average ridden in blocks is worth more than the average")
}

// The textbook formula on a fixed fixture: an hour at threshold is an intensity
// factor of one and a stress score of a hundred.
func TestPowerTSSMatchesTheTextbookFormula(t *testing.T) {
	t.Parallel()
	load, ok := trainingload.PowerLoad(steady(3601, 250), 250)
	require.True(t, ok)

	assert.InDelta(t, 1.0, load.IntensityFactor, 0.01)
	assert.InDelta(t, 100.0, load.TSS, 1.0)
}

func TestPowerLoadNeedsAThresholdPower(t *testing.T) {
	t.Parallel()
	_, ok := trainingload.PowerLoad(steady(3601, 250), 0)
	assert.False(t, ok, "no threshold, no intensity to measure")
}

// A ride shorter than the rolling window has no rolling mean to raise, so it
// yields nothing rather than its own average dressed up as normalized power.
func TestPowerLoadNeedsMoreThanTheRollingWindow(t *testing.T) {
	t.Parallel()
	_, ok := trainingload.PowerLoad(steady(20, 250), 250)
	assert.False(t, ok)
}

func TestDeriveYieldsOnlyWhatTheSensorsAndProfileAllow(t *testing.T) {
	t.Parallel()
	heartRate := steady(3601, 150)
	power := steady(3601, 200)

	full := trainingload.Derive(heartRate, power, 3600, trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 50,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
	})
	assert.True(t, full.HasZones && full.HasTRIMP && full.HasHeartRateTSS && full.HasPower)
	assert.True(t, full.Derived())

	heartOnly := trainingload.Derive(heartRate, nil, 3600, trainingload.Inputs{MaxHeartRateBPM: 190})
	assert.True(t, heartOnly.HasZones, "a maximum alone still cuts zones")
	assert.False(t, heartOnly.HasTRIMP, "but without a resting rate there is no reserve")
	assert.False(t, heartOnly.HasHeartRateTSS, "and without a threshold no stress score")
	assert.False(t, heartOnly.HasPower, "and no ride carried a meter")

	nothing := trainingload.Derive(nil, nil, 3600, trainingload.Inputs{})
	assert.False(t, nothing.Derived(), "no sensor and no profile yields no row at all")
}

// A strap that held for a fifth of the ride's moving time must not leave a
// TRIMP, a stress score or zones behind it, understated but unmarked.
func TestDeriveWithholdsTheHeartRateFiguresBelowMinSeriesCoverage(t *testing.T) {
	t.Parallel()
	inputs := trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 50,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
	}

	partial := trainingload.Derive(steady(600, 150), steady(3601, 200), 3600, inputs)
	assert.False(t, partial.HasZones, "the strap held for a fifth of the ride's moving time")
	assert.Zero(t, partial.Zones, "LoadOf reads Zones unconditionally, so a withheld ride must carry none")
	assert.False(t, partial.HasTRIMP)
	assert.False(t, partial.HasHeartRateTSS)
	assert.True(t, partial.HasPower, "the meter's own coverage is unaffected by the strap's")
}

func TestDeriveWithholdsThePowerFiguresBelowMinSeriesCoverage(t *testing.T) {
	t.Parallel()
	inputs := trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 50,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
	}

	partial := trainingload.Derive(steady(3601, 150), steady(600, 200), 3600, inputs)
	assert.False(t, partial.HasPower, "the meter held for a fifth of the ride's moving time")
	assert.True(t, partial.HasZones && partial.HasTRIMP && partial.HasHeartRateTSS,
		"the strap's own coverage is unaffected by the meter's")

	full := trainingload.Derive(steady(3601, 150), steady(3601, 200), 3600, inputs)
	assert.True(t, full.HasPower)
}

// The coverage share is served beside a figure whether or not it cleared the
// threshold: a served figure at 92% is still not the whole ride, and a
// withheld one carries the share a reader would use to judge how close it
// came, not just the fact that it fell short.
func TestDeriveKeepsCoverageAlongsideBothServedAndWithheldFigures(t *testing.T) {
	t.Parallel()
	inputs := trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 50,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
	}

	served := trainingload.Derive(steady(3601, 150), steady(3601, 200), 3600, inputs)
	require.True(t, served.HasHeartRateCoverage && served.HasPowerCoverage)
	assert.InDelta(t, 1.0, served.HeartRateCoverage, 0.001)
	assert.InDelta(t, 1.0, served.PowerCoverage, 0.001)

	withheld := trainingload.Derive(steady(600, 150), steady(600, 200), 3600, inputs)
	require.True(t, withheld.HasHeartRateCoverage && withheld.HasPowerCoverage)
	assert.False(t, withheld.HasTRIMP, "the withheld figure is still gone")
	assert.InDelta(t, 600.0/3600.0, withheld.HeartRateCoverage, 0.001,
		"but the share it fell short at is still reported")
	assert.InDelta(t, 600.0/3600.0, withheld.PowerCoverage, 0.001)
}

// Without a moving time to judge coverage against, today's behaviour holds:
// nothing is withheld that the sensors and profile would otherwise allow.
func TestDeriveWithholdsNothingWhenMovingSecondsIsUnknown(t *testing.T) {
	t.Parallel()
	full := trainingload.Derive(steady(600, 150), steady(600, 200), 0, trainingload.Inputs{
		MaxHeartRateBPM: 190, RestingHeartRateBPM: 50,
		ThresholdHeartRateBPM: 170, FunctionalThresholdPowerWatts: 250,
	})
	assert.True(t, full.HasZones && full.HasTRIMP && full.HasHeartRateTSS && full.HasPower)
	assert.False(t, full.HasHeartRateCoverage || full.HasPowerCoverage,
		"nothing to share a coverage of without a moving time")
}

func TestInputsOfReadsTheFourParametersADerivationUses(t *testing.T) {
	t.Parallel()
	profile := rider.Profile{
		MaxHeartRateBPM:               rider.Set(190),
		FunctionalThresholdPowerWatts: rider.Set(250),
		RiderMassKG:                   rider.Set(74),
	}
	inputs := trainingload.InputsOf(&profile)

	assert.InDelta(t, 190.0, inputs.MaxHeartRateBPM, 1e-9)
	assert.InDelta(t, 250.0, inputs.FunctionalThresholdPowerWatts, 1e-9)
	assert.Zero(t, inputs.ThresholdHeartRateBPM, "a parameter the rider has not entered reads as absent")
}
