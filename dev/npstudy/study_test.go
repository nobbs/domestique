package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
)

func start() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

func series(values ...float64) []measure.Reading {
	readings := make([]measure.Reading, len(values))
	for index, value := range values {
		readings[index] = measure.Reading{At: start().Add(time.Duration(index) * time.Second), Value: value}
	}

	return readings
}

// repeated is values cycled for seconds seconds.
func repeated(seconds int, values ...float64) []measure.Reading {
	cycled := make([]float64, seconds)
	for index := range cycled {
		cycled[index] = values[index%len(values)]
	}

	return series(cycled...)
}

func TestSmoothedIsTheTrailingMeanWithinTheWindow(t *testing.T) {
	t.Parallel()
	got := smoothed(series(0, 10, 20, 30), 2*time.Second)

	values := make([]float64, len(got))
	for index, reading := range got {
		values[index] = reading.Value
	}
	assert.Equal(t, []float64{0, 5, 15, 25}, values)
}

func TestSmoothedRestartsAfterARecordingGap(t *testing.T) {
	t.Parallel()
	readings := series(100, 100)
	readings = append(readings, measure.Reading{At: start().Add(time.Minute), Value: 0})

	got := smoothed(readings, time.Hour)

	assert.InDelta(t, 0, got[2].Value, 1e-9, "the reading after the gap is not averaged with those before it")
}

func TestFiguresOfASteadySeriesPutNPAtTheMean(t *testing.T) {
	t.Parallel()
	figures, ok := figuresOf(repeated(600, 200))

	require.True(t, ok)
	assert.InDelta(t, 200, figures.mean, 1e-6)
	assert.InDelta(t, 200, figures.np, 1e-6)
	assert.InDelta(t, 200, figures.smoothNP[1], 1e-6)
}

// A minute on, a minute off holds NP above the mean through 10 s smoothing,
// and less of it survives 60 s.
func TestFiguresOfSeparateNoiseFromSurgesBySmoothing(t *testing.T) {
	t.Parallel()
	surging := make([]float64, 0, 1200)
	for index := range 1200 {
		surging = append(surging, map[bool]float64{true: 300, false: 100}[index/60%2 == 0])
	}

	figures, ok := figuresOf(series(surging...))

	require.True(t, ok)
	assert.InDelta(t, 200, figures.mean, 1)
	assert.Greater(t, figures.np, 1.15*figures.mean)
	assert.Greater(t, figures.smoothNP[0], 1.1*figures.mean)
	assert.Less(t, figures.smoothNP[1], figures.smoothNP[0])
}

func TestFiguresOfRefusesASeriesWithNoPower(t *testing.T) {
	t.Parallel()
	_, ok := figuresOf(repeated(600, 0))
	assert.False(t, ok)
}

func TestStravaWattsAtTakesTheNearestActivityWithinTheWindow(t *testing.T) {
	t.Parallel()
	activities := []stravaActivity{
		{start: start().Add(90 * time.Second), averageWatts: 150},
		{start: start().Add(-30 * time.Second), averageWatts: 180},
		{start: start().Add(10 * time.Second), averageWatts: 0}, // no power recorded
		{start: start().Add(time.Hour), averageWatts: 999},
	}

	watts, ok := stravaWattsAt(activities, start())
	_, far := stravaWattsAt(activities, start().Add(30*time.Minute))

	require.True(t, ok)
	assert.InDelta(t, 180, watts, 1e-9)
	assert.False(t, far)
}

func TestParseStravaActivitiesReadsDateAndPowerByHeader(t *testing.T) {
	t.Parallel()
	csv := "Activity ID,Activity Date,Distance,Average Watts,Distance\n" +
		"1,\"Aug 24, 2026, 6:00:00 AM\",40.1,151.5,40100\n" +
		"2,\"Aug 25, 2026, 6:00:00 AM\",12.0,,12000\n" +
		"3,not a date,1,100,1\n"

	activities, err := parseStravaActivities(strings.NewReader(csv))

	require.NoError(t, err)
	require.Len(t, activities, 1)
	assert.Equal(t, start(), activities[0].start)
	assert.InDelta(t, 151.5, activities[0].averageWatts, 1e-9)
}

func TestParseStravaActivitiesRefusesAnExportWithoutPower(t *testing.T) {
	t.Parallel()
	_, err := parseStravaActivities(strings.NewReader("Activity ID,Activity Date\n1,x\n"))
	assert.ErrorContains(t, err, "Average Watts")
}

// A trainer ride whose meter is the model itself at a known drag area must
// fit back to that drag area.
func TestFitTrainerDragAreaRecoversTheBicycleTheMeterRodeAt(t *testing.T) {
	t.Parallel()
	truth := measure.Coefficients{DragArea: 0.30, RollingResistance: 0.004}
	var rides []trainerRide
	for _, metresPerSecond := range []float64{6, 9, 12} {
		track := make([]measure.Sample, 0, 900)
		for second := range 900 {
			track = append(track, measure.Sample{
				At: start().Add(time.Duration(second) * time.Second), DistanceMetres: metresPerSecond * float64(second),
			})
		}
		estimates, ok := measure.EstimateSeries(track, 80, truth)
		require.True(t, ok)
		meter, figuresOK := figuresOf(estimatedReadings(track, estimates))
		require.True(t, figuresOK)
		rides = append(rides, trainerRide{track: track, meter: meter, mass: 80})
	}

	fitted, ok := fitTrainerDragArea(rides, truth.RollingResistance)

	require.True(t, ok)
	assert.InDelta(t, truth.DragArea, fitted.DragArea, 0.005)
}

func TestFitTrainerDragAreaRefusesNoRides(t *testing.T) {
	t.Parallel()
	_, ok := fitTrainerDragArea(nil, 0.004)
	assert.False(t, ok)
}

func TestReportListsRatiosInTheOrderAdded(t *testing.T) {
	t.Parallel()
	r := report{ratios: map[string][]float64{}}
	r.add("second", 1, 0) // no denominator: never a ratio
	r.add("first", 3, 2)
	r.add("first", 1, 1)

	out := r.String()

	assert.Contains(t, out, "first")
	assert.NotContains(t, out, "second")
	assert.Contains(t, out, "trainer bicycle: unavailable")
}
