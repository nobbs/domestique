package trainingload_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutlookOfIsAbsentForAnEmptyTimeline(t *testing.T) {
	t.Parallel()
	_, ok := trainingload.OutlookOf(nil)
	assert.False(t, ok)
}

// The acceptance criterion: the ramp is exactly the fitness difference a
// reader could read off the timeline itself, seven days apart.
func TestOutlookOfRampMatchesTheSevenDayFitnessDifference(t *testing.T) {
	t.Parallel()
	rides := make([]trainingload.RideLoad, 0, 20)
	for index := range 20 {
		rides = append(rides, trainingload.RideLoad{At: day(2026, time.June, 1+index), TSS: 40})
	}
	timeline := trainingload.Timeline(rides, day(2026, time.June, 20), berlin(t))

	outlook, ok := trainingload.OutlookOf(timeline)
	require.True(t, ok)

	last := timeline[len(timeline)-1]
	earlier := timeline[len(timeline)-8]
	assert.InDelta(t, last.TSSFitness-earlier.TSSFitness, outlook.TSS.RampPerWeek, 1e-9)
}

// A timeline too short to hold a day seven days before its last folds from
// zero, the same starting point the timeline itself folds from.
func TestOutlookOfRampFoldsFromZeroOnAShortTimeline(t *testing.T) {
	t.Parallel()
	rides := []trainingload.RideLoad{{At: day(2026, time.June, 1), TSS: 40}}
	timeline := trainingload.Timeline(rides, day(2026, time.June, 5), berlin(t))
	require.Len(t, timeline, 5, "5 days, none of them 8 apart")

	outlook, ok := trainingload.OutlookOf(timeline)
	require.True(t, ok)
	last := timeline[len(timeline)-1]
	assert.InDelta(t, last.TSSFitness, outlook.TSS.RampPerWeek, 1e-9)
}

// An eight-day timeline has a day exactly seven before its last, and the ramp reads it.
func TestOutlookOfRampReadsTheDaySevenBeforeOnAnEightDayTimeline(t *testing.T) {
	t.Parallel()
	days := make([]trainingload.Day, 8)
	for index := range days {
		days[index] = trainingload.Day{Date: day(2026, time.June, 1+index), TSSFitness: float64(10 + index)}
	}

	outlook, ok := trainingload.OutlookOf(days)
	require.True(t, ok)
	assert.InDelta(t, 7.0, outlook.TSS.RampPerWeek, 1e-9)
}

// The acceptance criterion: the habitual load is always the last 28 days'
// total divided by 28, even when the timeline holds fewer than that.
func TestOutlookOfHabitualLoadDividesByTwentyEightRegardless(t *testing.T) {
	t.Parallel()
	days := make([]trainingload.Day, 10)
	for index := range days {
		days[index] = trainingload.Day{Date: day(2026, time.June, 1+index), TSSLoad: 28}
	}

	outlook, ok := trainingload.OutlookOf(days)
	require.True(t, ok)
	assert.InDelta(t, 280.0/28.0, outlook.TSS.HabitualDailyLoad, 1e-9)
}

// The acceptance criterion for the weekly range: spreading it evenly over the
// next seven days actually raises fitness by the promised share.
func TestOutlookOfWeeklyRangeRaisesFitnessByItsShare(t *testing.T) {
	t.Parallel()
	last := trainingload.Day{Date: day(2026, time.June, 30), TSSFitness: 100, TSSFatigue: 40}
	outlook, ok := trainingload.OutlookOf([]trainingload.Day{last})
	require.True(t, ok)

	assert.InDelta(t, 100*1.03, foldSevenDays(last.TSSFitness, outlook.TSS.WeekLoadLow/7), 1e-6,
		"the low end raises fitness by 3%")
	assert.InDelta(t, 100*1.08, foldSevenDays(last.TSSFitness, outlook.TSS.WeekLoadHigh/7), 1e-6,
		"the high end raises fitness by 8%")
}

// foldSevenDays is the same recurrence the timeline folds with, written out by
// hand: the acceptance criterion the weekly range is measured against.
func foldSevenDays(fitness, dailyLoad float64) float64 {
	for range 7 {
		fitness += (dailyLoad - fitness) / trainingload.FitnessDays
	}

	return fitness
}

// Rest carries no load, so both averages decay towards zero — and fatigue,
// decaying seven times as fast as fitness, falls further, which is what makes
// form rise under rest.
func TestOutlookOfRestProjectionDecaysAndFormRises(t *testing.T) {
	t.Parallel()
	last := trainingload.Day{Date: day(2026, time.June, 30), TSSFitness: 50, TSSFatigue: 50}
	outlook, ok := trainingload.OutlookOf([]trainingload.Day{last})
	require.True(t, ok)

	rest := outlook.TSS.Rest
	require.Len(t, rest, trainingload.OutlookDays)
	assert.Less(t, rest[len(rest)-1].Fitness, last.TSSFitness, "fitness decays towards zero")
	assert.Less(t, rest[len(rest)-1].Fatigue, rest[0].Fatigue, "fatigue keeps falling")
	assert.Greater(t, rest[len(rest)-1].Form, 0.0, "fresher every day under no load")
	assert.InDelta(t, rest[0].Fitness-rest[0].Fatigue, rest[0].Form, 1e-9)
}

// The build plan projects a fifth more load than the habitual plan, so it
// diverges from the habitual daily load projection.
func TestOutlookOfBuildProjectsMoreThanHabitual(t *testing.T) {
	t.Parallel()
	days := make([]trainingload.Day, 30)
	for index := range days {
		days[index] = trainingload.Day{Date: day(2026, time.June, 1+index), TSSLoad: 20}
	}
	days[len(days)-1].TSSFitness, days[len(days)-1].TSSFatigue = 5, 5

	outlook, ok := trainingload.OutlookOf(days)
	require.True(t, ok)
	assert.Greater(t, outlook.TSS.Build[0].Fitness, outlook.TSS.Habitual[0].Fitness,
		"the higher daily load raises fitness further on the very first day")
}

// Every plan projects exactly 21 days, one after another with no gaps.
func TestOutlookOfProjectsTwentyOneConsecutiveDays(t *testing.T) {
	t.Parallel()
	last := trainingload.Day{Date: day(2026, time.June, 30), TSSFitness: 50, TSSFatigue: 20}
	outlook, ok := trainingload.OutlookOf([]trainingload.Day{last})
	require.True(t, ok)

	for _, plan := range [][]trainingload.ProjectedDay{outlook.TSS.Rest, outlook.TSS.Habitual, outlook.TSS.Build} {
		require.Len(t, plan, trainingload.OutlookDays)
		for index, projected := range plan {
			assert.Equal(t, last.Date.AddDate(0, 0, index+1), projected.Date)
		}
	}
}

// TSS and TRIMP each read their own series: nothing here is shared between
// the two scales.
func TestOutlookOfReadsEachScalesOwnSeries(t *testing.T) {
	t.Parallel()
	last := trainingload.Day{
		Date:         day(2026, time.June, 30),
		TSSLoad:      10,
		TSSFitness:   10,
		TSSFatigue:   5,
		TRIMPLoad:    90,
		TRIMPFitness: 90,
		TRIMPFatigue: 45,
	}
	outlook, ok := trainingload.OutlookOf([]trainingload.Day{last})
	require.True(t, ok)

	assert.InDelta(t, 10.0/28.0, outlook.TSS.HabitualDailyLoad, 1e-9)
	assert.InDelta(t, 90.0/28.0, outlook.TRIMP.HabitualDailyLoad, 1e-9)
	assert.InDelta(t, 10.0, outlook.TSS.RampPerWeek, 1e-9)
	assert.InDelta(t, 90.0, outlook.TRIMP.RampPerWeek, 1e-9)
}
