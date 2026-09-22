package activity

import (
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func minimalBundle() *bundle {
	return &bundle{
		metrics:  &RideMetrics{},
		location: time.UTC,
		at:       time.Date(2026, 9, 13, 18, 0, 0, 0, time.UTC),
	}
}

func TestComposeCarriesTheProfileAndPowerCurve(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.profile = rider.Profile{
		MaxHeartRateBPM: rider.Value{Number: 190, Set: true}, ThresholdHeartRateBPM: rider.Value{Number: 170, Set: true},
	}
	b.powerCurve = rider.PowerCurve{
		Watts: [rider.PowerCurvePoints]float64{820}, Held: [rider.PowerCurvePoints]bool{true},
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "Rider:")
	assert.Contains(t, prompt, "maximum heart rate 190 bpm")
	assert.Contains(t, prompt, "best power ever: 5 s 820 W")
}

func TestComposeOmitsRiderWhenThereIsNothingToSay(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, minimalBundle().compose(), "Rider:")
}

func TestComposeCarriesThisRideAndOmitsCoordinatesAndSubject(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.ride = Stored{
		StartedAt: time.Date(2026, 9, 13, 8, 0, 0, 0, time.UTC), Provider: "wahoo",
		DistanceMetres: 42000, MovingSeconds: 5400, ElapsedSeconds: 5600, AscentMetres: 350,
	}
	b.session = Session{AveragePowerWatts: Reading{Value: 210, Known: true}}
	b.match = &RouteMatch{RouteCoverage: 0.95, RideCoverage: 0.9, Direction: DirectionForward}
	b.routeName = "Alpe secrète"
	b.weather = &WeatherSummary{TemperatureMinCelsius: 10, TemperatureMaxCelsius: 18, WeatherCode: 1}
	at := b.ride.StartedAt
	b.series = []SampleRow{{
		Time: at, HeartRateBPM: Reading{Value: 140, Known: true}, AltitudeMetres: Reading{Value: 512, Known: true},
		Latitude: Reading{Value: 47.123456, Known: true}, Longitude: Reading{Value: 8.654321, Known: true},
	}}

	prompt := b.compose()
	assert.Contains(t, prompt, "This ride:")
	assert.Contains(t, prompt, ",512,")
	assert.NotContains(t, prompt, "47.12")
	assert.NotContains(t, prompt, "8.65")
	assert.Contains(t, prompt, "distance 42.0 km")
	assert.Contains(t, prompt, "average power 210 W")
	assert.Contains(t, prompt, "library route: Alpe secrète, direction forward")
	assert.Contains(t, prompt, "weather: 10.0 to 18.0")
	assert.NotContains(t, prompt, "latitude")
	assert.NotContains(t, prompt, "longitude")
	assert.NotContains(t, prompt, "subject")
}

func TestComposeCarriesEarlierAnalysesWithNewestFirst(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	newest := AnalysisDocument{Headline: "A steady grind", Summary: "Line one.\nLine two."}
	newest.NextSession.Advice = "Rest tomorrow."
	b.earlier = []Analysis{
		{StartedAt: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), Document: newest},
		{StartedAt: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), Text: "an older plain-text row"},
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "What was said about this rider's earlier rides, newest first:")
	assert.Contains(t, prompt, "2026-09-10: A steady grind — Line one. Line two. Next: Rest tomorrow.")
	assert.Contains(t, prompt, "2026-09-05: an older plain-text row")
}

func TestComposeOmitsEarlierWhenThereIsNone(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, minimalBundle().compose(), "earlier rides")
}

// Splits: fewer than two stretches emit no table at all.
func TestSplitsSectionIsOmittedUnderTwoRows(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.series = []SampleRow{{Time: b.at, DistanceMetres: Reading{Value: 0, Known: true}}}

	assert.NotContains(t, b.compose(), "Splits:")
}

func TestSplitsSectionCarriesEachStretch(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	origin := b.at
	b.series = []SampleRow{
		{Time: origin, DistanceMetres: Reading{Value: 0, Known: true}, HeartRateBPM: Reading{Value: 140, Known: true}},
		{Time: origin.Add(time.Minute), DistanceMetres: Reading{Value: 5000, Known: true}, HeartRateBPM: Reading{Value: 150, Known: true}},
		{Time: origin.Add(2 * time.Minute), DistanceMetres: Reading{Value: 8000, Known: true}},
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "Splits:\nsplit_km,distance_km,moving_min,ascent_m,avg_hr,avg_power")
	assert.Contains(t, prompt, "5.0,5.0,1.0,0,150,")
	assert.Contains(t, prompt, "8.0,3.0,1.0,0,,")
}

// Climbs: a route with climbs but no attempt of this ride's own is omitted;
// best and median are read over every other stored attempt, excluding this ride.
func TestClimbsSectionExcludesThisRidesOwnAttemptFromBestAndMedian(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.climbs = []measure.Climb{{DistanceMetres: 2000, AscentMetres: 150}}
	b.thisAttempts = map[int]ClimbAttempt{0: {Seconds: 600, HeartRateBPM: 160, HasHeartRate: true}}
	b.otherAttempts = []StoredClimbAttempt{
		{ClimbAttempt: ClimbAttempt{ClimbIndex: 0, Seconds: 500}}, //nolint:modernize // Explicit type keeps the three rows scannable.
		{ClimbAttempt: ClimbAttempt{ClimbIndex: 0, Seconds: 700}}, //nolint:modernize // Explicit type keeps the three rows scannable.
		{ClimbAttempt: ClimbAttempt{ClimbIndex: 0, Seconds: 900}}, //nolint:modernize // Explicit type keeps the three rows scannable.
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "Climbs:")
	assert.Contains(t, prompt, "1,2.0,150,10.0,160,,8.3,11.7,3")
}

func TestClimbsSectionOmittedWithoutAnAttemptOfThisRide(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.climbs = []measure.Climb{{DistanceMetres: 2000, AscentMetres: 150}}

	assert.NotContains(t, b.compose(), "Climbs:")
}

// Structured workout: consecutive samples at the same target within 1 W group
// into one interval; an interval under thirty seconds is dropped.
func TestWorkoutSectionGroupsConsecutiveTargetsAndDropsShortOnes(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	origin := b.at
	b.series = []SampleRow{
		{Time: origin, TargetPowerWatts: Reading{Value: 200, Known: true}, PowerWatts: Reading{Value: 195, Known: true}},
		{Time: origin.Add(20 * time.Second), TargetPowerWatts: Reading{Value: 200.4, Known: true}, PowerWatts: Reading{Value: 205, Known: true}},
		{Time: origin.Add(40 * time.Second), TargetPowerWatts: Reading{Value: 200, Known: true}, PowerWatts: Reading{Value: 200, Known: true}},
		{Time: origin.Add(50 * time.Second), TargetPowerWatts: Reading{Value: 300, Known: true}, PowerWatts: Reading{Value: 295, Known: true}},
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "Structured workout:")
	assert.Contains(t, prompt, "0.0,0.7,200,200,0,")
	assert.NotContains(t, prompt, "300,295")
}

func TestWorkoutSectionOmittedWithNoTargetPower(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.series = []SampleRow{{Time: b.at, PowerWatts: Reading{Value: 200, Known: true}}}

	assert.NotContains(t, b.compose(), "Structured workout:")
}

// Weather steps: one CSV row per step, in local time.
func TestWeatherStepsSection(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.weatherSteps = []WeatherStep{
		{At: time.Date(2026, 9, 13, 8, 15, 0, 0, time.UTC), TemperatureCelsius: 12, WeatherCode: 61},
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "Weather during the ride:")
	assert.Contains(t, prompt, "08:15,12.0")
	assert.Contains(t, prompt, "drizzle/rain")
}

func TestWeatherStepsSectionOmittedWhenEmpty(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, minimalBundle().compose(), "Weather during the ride")
}

// Timeseries: 5 s buckets mean the known readings, keep the last known
// distance and altitude, blank an unmeasured cell, and stop at six hours.
func TestTimeseriesBucketsMeanAndLastKnownValueAndBlanksTheUnmeasured(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	origin := b.at
	b.series = []SampleRow{
		{
			Time: origin, DistanceMetres: Reading{Value: 0, Known: true}, HeartRateBPM: Reading{Value: 140, Known: true},
			AltitudeMetres: Reading{Value: 100, Known: true},
		},
		{
			Time: origin.Add(2 * time.Second), DistanceMetres: Reading{Value: 10, Known: true},
			HeartRateBPM: Reading{Value: 150, Known: true}, AltitudeMetres: Reading{Value: 105, Known: true},
		},
		{Time: origin.Add(6 * time.Second), DistanceMetres: Reading{Value: 20, Known: true}},
	}

	prompt := b.compose()
	assert.Contains(t, prompt, "Timeseries:\nt_s,dist_km,alt_m,grade_pct,speed_kmh,hr,power,est_power,cadence,temp_c,target_w,tailwind_kmh")
	assert.Contains(t, prompt, "0,0.0,105,,18.0,145,,,,,,")
	assert.Contains(t, prompt, "5,0.0,,,9.0,,,,,,,")
}

func TestTimeseriesTruncatesAtSixHours(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	origin := b.at
	rows := make([]SampleRow, 0, maximumTimeseriesRows+5)
	for i := range maximumTimeseriesRows + 5 {
		at := origin.Add(time.Duration(i) * timeseriesStep)
		rows = append(rows, SampleRow{Time: at, HeartRateBPM: Reading{Value: 140, Known: true}})
	}
	b.series = rows

	prompt := b.compose()
	assert.Contains(t, prompt, "(timeseries truncated at six hours)")
}

// Training load: the fitness/fatigue/form lines, then the timeline windows;
// each is omitted when its source yields nothing.
func TestTrainingLoadSection(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.day = &trainingload.Day{TSSFitness: 62, TSSFatigue: 71, TSSForm: -9}
	b.loadLabel = loadNow
	b.loads = []trainingload.RideLoad{{At: b.at.Add(-time.Hour), TSS: 80, TRIMP: 90}}

	prompt := b.compose()
	assert.Contains(t, prompt, loadNow+":")
	assert.Contains(t, prompt, "Training load, last 42 days:")
	assert.Contains(t, prompt, "Outlook:")
}

func TestTrainingLoadSectionOmittedWithoutADay(t *testing.T) {
	t.Parallel()
	assert.NotContains(t, minimalBundle().compose(), loadNow)
}

// Recent rides: this ride itself is excluded from its own recent-rides table.
func TestRecentRidesExcludesThisRide(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	b.ride = Stored{ID: 1, StartedAt: b.at.Add(-time.Hour)}
	b.recent = []Stored{
		{ID: 1, StartedAt: b.at.Add(-time.Hour), DistanceMetres: 1000},
		{ID: 2, StartedAt: b.at.Add(-24 * time.Hour), DistanceMetres: 20000, MovingSeconds: 3600},
	}
	b.recentByID = map[int64]RideMetrics{}

	prompt := b.compose()
	require.Contains(t, prompt, "Recent rides (newest first):")
	assert.Contains(t, prompt, "20.0,1.0,0")
	assert.NotContains(t, prompt, ",1.0,0.0,")
}

func TestClimbsMedianInterpolatesAnEvenCount(t *testing.T) {
	t.Parallel()
	_, median, count := climbStats([]StoredClimbAttempt{
		{ClimbAttempt: ClimbAttempt{Seconds: 700}}, //nolint:modernize // Explicit type keeps the rows scannable.
		{ClimbAttempt: ClimbAttempt{Seconds: 500}}, //nolint:modernize // Explicit type keeps the rows scannable.
	})
	assert.InDelta(t, 600, median, 0.001)
	assert.Equal(t, 2, count)
}

func TestWeatherCodeWordsFollowWMO(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "clear", weatherCodeWord(0))
	assert.Equal(t, "mainly clear", weatherCodeWord(1))
	assert.Equal(t, "overcast", weatherCodeWord(3))
	assert.Equal(t, "code 30", weatherCodeWord(30))
}

// Tailwind: the wind's component along a bucket's bearing, from the step that
// covers it; a bucket that did not move, or has no step, carries none.
func TestTimeseriesTailwindFollowsTheBearing(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	origin := b.at
	north := func(at time.Time, lat float64) SampleRow {
		return SampleRow{
			Time: at, Latitude: Reading{Value: lat, Known: true}, Longitude: Reading{Value: 8, Known: true},
			HeartRateBPM: Reading{Value: 140, Known: true},
		}
	}
	b.series = []SampleRow{
		north(origin, 50), north(origin.Add(4*time.Second), 50.001),
		north(origin.Add(5*time.Second), 50.001), north(origin.Add(9*time.Second), 50.002),
		north(origin.Add(10*time.Second), 50.002), north(origin.Add(14*time.Second), 50.002),
		north(origin.Add(60*time.Second), 50.002), north(origin.Add(64*time.Second), 50.003),
	}
	b.weatherSteps = []WeatherStep{
		{At: origin, Step: 5 * time.Second, WindSpeedKMH: 20, WindDirectionDegrees: 0},
		{At: origin.Add(5 * time.Second), Step: 10 * time.Second, WindSpeedKMH: 20, WindDirectionDegrees: 180},
	}

	prompt := b.compose()
	assert.NotContains(t, prompt, "50.00", "no coordinate leaves the host")
	assert.Contains(t, prompt, "\n0,,,,,140,,,,,,-20", "riding north into a north wind")
	assert.Contains(t, prompt, "\n5,,,,,140,,,,,,20", "riding north with a south wind behind")
	assert.Contains(t, prompt, "\n10,,,,,140,,,,,,\n", "a bucket that did not move has no bearing")
	assert.True(t, strings.HasSuffix(prompt, "\n60,,,,,140,,,,,,"), "a bucket outside every step has no wind")
}

func TestTimeseriesKeepsAPositionOnlyBucketForItsTailwind(t *testing.T) {
	t.Parallel()
	b := minimalBundle()
	origin := b.at
	b.series = []SampleRow{
		{Time: origin, Latitude: Reading{Value: 50, Known: true}, Longitude: Reading{Value: 8, Known: true}},
		{Time: origin.Add(4 * time.Second), Latitude: Reading{Value: 50.001, Known: true}, Longitude: Reading{Value: 8, Known: true}},
	}
	b.weatherSteps = []WeatherStep{{At: origin, Step: time.Minute, WindSpeedKMH: 10, WindDirectionDegrees: 180}}

	assert.Contains(t, b.compose(), "\n0,,,,,,,,,,,10")
}
