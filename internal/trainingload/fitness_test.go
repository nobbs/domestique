package trainingload_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func berlin(t *testing.T) *time.Location {
	t.Helper()
	location, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)

	return location
}

func day(year int, month time.Month, date int) time.Time {
	return time.Date(year, month, date, 9, 0, 0, 0, time.UTC)
}

// referenceFold is the recurrence written out by hand, which is what the
// acceptance criterion measures the implementation against.
func referenceFold(loads []float64, days float64) float64 {
	average := 0.0
	for _, load := range loads {
		average += (load - average) / days
	}

	return average
}

// The acceptance criterion: over a synthetic month the two averages match a
// reference fold.
func TestTimelineMatchesAReferenceFold(t *testing.T) {
	t.Parallel()
	rides := make([]trainingload.RideLoad, 0, 30)
	loads := make([]float64, 0, 30)
	for index := range 30 {
		load := float64(40 + index)
		rides = append(rides, trainingload.RideLoad{At: day(2026, time.June, 1+index), TSS: load})
		loads = append(loads, load)
	}

	timeline := trainingload.Timeline(rides, day(2026, time.June, 30), berlin(t))
	require.Len(t, timeline, 30, "one row per day")

	last := timeline[len(timeline)-1]
	assert.InDelta(t, referenceFold(loads, trainingload.FitnessDays), last.TSSFitness, 1e-9)
	assert.InDelta(t, referenceFold(loads, trainingload.FatigueDays), last.TSSFatigue, 1e-9)
	assert.InDelta(t, last.TSSFitness-last.TSSFatigue, last.TSSForm, 1e-9, "form is their difference")
}

// The acceptance criterion's other half: a gap of ride-free days decays both,
// and rest is what turns work into form.
func TestTimelineDecaysAcrossRideFreeDays(t *testing.T) {
	t.Parallel()
	rides := []trainingload.RideLoad{
		{At: day(2026, time.June, 1), TSS: 100},
		{At: day(2026, time.June, 2), TSS: 100},
	}

	timeline := trainingload.Timeline(rides, day(2026, time.June, 20), berlin(t))
	require.Len(t, timeline, 20, "the rest days are days too")

	ridden := timeline[1]
	rested := timeline[len(timeline)-1]
	assert.Less(t, rested.TSSFitness, ridden.TSSFitness, "fitness fades")
	assert.Less(t, rested.TSSFatigue, ridden.TSSFatigue, "and fatigue fades faster")
	assert.Negative(t, ridden.TSSForm, "buried on the second hard day")
	assert.Positive(t, rested.TSSForm, "and fresh after a fortnight off")
	assert.Zero(t, rested.TSSLoad, "a day nobody rode carries no load of its own")
}

// Two rides on one day are one day's load.
func TestTimelineSumsTheRidesOfOneDay(t *testing.T) {
	t.Parallel()
	rides := []trainingload.RideLoad{
		{At: time.Date(2026, time.June, 1, 8, 0, 0, 0, time.UTC), TSS: 40, TRIMP: 30},
		{At: time.Date(2026, time.June, 1, 17, 0, 0, 0, time.UTC), TSS: 60, TRIMP: 20},
	}

	timeline := trainingload.Timeline(rides, day(2026, time.June, 1), berlin(t))
	require.Len(t, timeline, 1)
	assert.InDelta(t, 100.0, timeline[0].TSSLoad, 1e-9)
	assert.InDelta(t, 50.0, timeline[0].TRIMPLoad, 1e-9)
}

// The fold starts at the first ride, not at the window: a rider who opens a
// window years into their history opens it at the fitness they had built.
func TestTimelineStartsAtTheFirstRideWhateverOrderTheyArriveIn(t *testing.T) {
	t.Parallel()
	rides := []trainingload.RideLoad{
		{At: day(2026, time.June, 10), TSS: 50},
		{At: day(2026, time.June, 1), TSS: 50},
	}

	timeline := trainingload.Timeline(rides, day(2026, time.June, 10), berlin(t))
	require.Len(t, timeline, 10)
	assert.Equal(t, 1, timeline[0].Date.Day(), "the earliest ride, whatever order it was given in")
}

func TestTimelineIsEmptyForARiderWithNoRides(t *testing.T) {
	t.Parallel()
	assert.Empty(t, trainingload.Timeline(nil, day(2026, time.June, 1), berlin(t)))
}

// A window ending before the first ride still yields that first day rather than
// nothing, so a caller never has to tell an empty series from a bad range.
func TestTimelineEndsNoEarlierThanItStarts(t *testing.T) {
	t.Parallel()
	timeline := trainingload.Timeline(
		[]trainingload.RideLoad{{At: day(2026, time.June, 10), TSS: 50}},
		day(2026, time.June, 1), berlin(t))

	require.Len(t, timeline, 1)
	assert.Equal(t, 10, timeline[0].Date.Day())
}

// The stress score follows the meter: power where the ride carried one, heart
// rate otherwise, so the series does not step on the day a meter arrives.
func TestLoadOfPrefersMeasuredPowerOverHeartRate(t *testing.T) {
	t.Parallel()
	at := day(2026, time.June, 1)

	withMeter := trainingload.Metrics{
		HasPower: true, Power: trainingload.Power{TSS: 80},
		HasHeartRateTSS: true, HeartRateTSS: 70,
		HasTRIMP: true, TRIMP: 60,
	}
	assert.InDelta(t, 80.0, trainingload.LoadOf(at, &withMeter).TSS, 1e-9)
	assert.InDelta(t, 60.0, trainingload.LoadOf(at, &withMeter).TRIMP, 1e-9)

	withStrap := trainingload.Metrics{HasHeartRateTSS: true, HeartRateTSS: 70}
	assert.InDelta(t, 70.0, trainingload.LoadOf(at, &withStrap).TSS, 1e-9)

	withNeither := trainingload.Metrics{}
	assert.Zero(t, trainingload.LoadOf(at, &withNeither).TSS)
	assert.Zero(t, trainingload.LoadOf(at, &withNeither).TRIMP)
}

// An estimate worked out from the track is never a stress score.
func TestLoadOfIgnoresEstimatedPower(t *testing.T) {
	t.Parallel()
	estimated := trainingload.Metrics{HasEstimatedPower: true, EstimatedPowerWatts: 210}

	assert.Zero(t, trainingload.LoadOf(day(2026, time.June, 1), &estimated).TSS)
}

// A training week starts on a Monday, whatever day the ride fell on.
func TestZonesByWeekSumsIntoMondays(t *testing.T) {
	t.Parallel()
	// The 3rd of June 2026 is a Wednesday; the 8th the Monday after.
	rides := []trainingload.RideLoad{
		{At: day(2026, time.June, 3), Zones: trainingload.Zones{60, 0, 0, 0, 0}},
		{At: day(2026, time.June, 7), Zones: trainingload.Zones{30, 0, 0, 0, 0}},
		{At: day(2026, time.June, 8), Zones: trainingload.Zones{0, 120, 0, 0, 0}},
	}

	weeks := trainingload.ZonesByWeek(rides, berlin(t))
	require.Len(t, weeks, 2, "the two weeks the rides fell in")
	assert.Equal(t, time.Monday, weeks[0].WeekStart.Weekday())
	assert.Equal(t, 1, weeks[0].WeekStart.Day(), "the Monday before the Wednesday")
	assert.InDelta(t, 90.0, weeks[0].Zones[0], 1e-9, "both rides of that week")
	assert.Equal(t, 8, weeks[1].WeekStart.Day())
	assert.InDelta(t, 120.0, weeks[1].Zones[1], 1e-9)
}

func TestZonesByWeekOrdersOldestFirstWhateverOrderTheyArriveIn(t *testing.T) {
	t.Parallel()
	rides := []trainingload.RideLoad{
		{At: day(2026, time.June, 20), Zones: trainingload.Zones{10, 0, 0, 0, 0}},
		{At: day(2026, time.June, 3), Zones: trainingload.Zones{20, 0, 0, 0, 0}},
		{At: day(2026, time.June, 12), Zones: trainingload.Zones{30, 0, 0, 0, 0}},
	}

	weeks := trainingload.ZonesByWeek(rides, berlin(t))
	require.Len(t, weeks, 3)
	assert.True(t, weeks[0].WeekStart.Before(weeks[1].WeekStart))
	assert.True(t, weeks[1].WeekStart.Before(weeks[2].WeekStart))
}

func TestZonesByWeekIsEmptyForNoRides(t *testing.T) {
	t.Parallel()
	assert.Empty(t, trainingload.ZonesByWeek(nil, berlin(t)))
}
