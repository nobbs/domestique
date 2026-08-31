package task

import (
	"testing"
	"time"

	// A calendar schedule is asserted against real zones, so the database has to
	// travel with the test binary. The environments this runs in are not
	// guaranteed to carry one, the same reason the service embeds it.
	_ "time/tzdata"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reference is a fixed instant, so a schedule's arithmetic is readable rather
// than relative to whenever the suite runs.
func reference() time.Time {
	return time.Date(2026, time.August, 30, 9, 0, 0, 0, time.UTC)
}

func TestEveryFiresOneGapAfterThePreviousRun(t *testing.T) {
	t.Parallel()

	schedule := Every(func() time.Duration { return time.Hour })

	assert.Equal(t, reference().Add(time.Hour), schedule.NextFire(reference()), "NextFire()")
}

func TestEveryNeverFiresAgainWithoutAPositiveGap(t *testing.T) {
	t.Parallel()

	for name, gap := range map[string]time.Duration{"zero": 0, "negative": -time.Minute} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.True(t, Every(func() time.Duration { return gap }).NextFire(reference()).IsZero(), "NextFire()")
		})
	}
}

func TestNextDueWaitsOutAGapStillAhead(t *testing.T) {
	t.Parallel()

	due, scheduled := nextDue(Every(func() time.Duration { return time.Hour }), reference(), reference())
	assert.True(t, scheduled, "nextDue() reported no schedule")
	assert.Equal(t, reference().Add(time.Hour), due, "nextDue()")
}

func TestNextDueStartsAtOnceWhenTheGapHasElapsed(t *testing.T) {
	t.Parallel()

	now := reference().Add(3 * time.Hour)
	due, scheduled := nextDue(Every(func() time.Duration { return time.Hour }), reference(), now)
	assert.True(t, scheduled, "nextDue() reported no schedule")
	assert.Equal(t, now, due, "nextDue()")
}

func TestNextDueReportsAScheduleThatWillNotFireAgain(t *testing.T) {
	t.Parallel()

	_, scheduled := nextDue(Every(func() time.Duration { return 0 }), reference(), reference())
	assert.False(t, scheduled, "nextDue() scheduled a run without a gap")
}

func TestDailyFiresAtItsWallClockTimeInTheServiceZone(t *testing.T) {
	t.Parallel()

	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err, "LoadLocation()")
	schedule := Daily{Hour: 2, Minute: 30, Zone: func() *time.Location { return berlin }}

	// 00:15 Berlin on the 30th, expressed in UTC as the manager holds it.
	previous := time.Date(2026, time.August, 29, 22, 15, 0, 0, time.UTC)

	next := schedule.NextFire(previous)
	assert.Equal(t, time.Date(2026, time.August, 30, 2, 30, 0, 0, berlin), next.In(berlin), "next fire")
}

func TestDailyMovesToTheNextDayOnceTodaysTimeHasPassed(t *testing.T) {
	t.Parallel()

	utc := Daily{Hour: 2, Minute: 0}
	previous := time.Date(2026, time.August, 30, 2, 0, 0, 0, time.UTC)

	assert.Equal(t,
		time.Date(2026, time.August, 31, 2, 0, 0, 0, time.UTC),
		utc.NextFire(previous), "next fire")
}

func TestWeeklyFiresOnItsOwnWeekday(t *testing.T) {
	t.Parallel()

	// 2026-08-30 is a Sunday, so the next Monday is the 31st.
	schedule := Weekly{Weekday: time.Monday, Hour: 2, Minute: 0}
	previous := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)

	next := schedule.NextFire(previous)
	assert.Equal(t, time.Monday, next.Weekday(), "weekday")
	assert.Equal(t, time.Date(2026, time.August, 31, 2, 0, 0, 0, time.UTC), next, "next fire")
}

func TestWeeklyWaitsAWholeWeekFromItsOwnFiring(t *testing.T) {
	t.Parallel()

	schedule := Weekly{Weekday: time.Monday, Hour: 2, Minute: 0}
	previous := time.Date(2026, time.August, 31, 2, 0, 0, 0, time.UTC)

	assert.Equal(t,
		time.Date(2026, time.September, 7, 2, 0, 0, 0, time.UTC),
		schedule.NextFire(previous), "next fire")
}

func TestADailyScheduleRollsForwardThroughASkippedHour(t *testing.T) {
	t.Parallel()

	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err, "LoadLocation()")
	// Europe/Berlin skips 02:00–03:00 on 2026-03-29.
	schedule := Daily{Hour: 2, Minute: 30, Zone: func() *time.Location { return berlin }}
	previous := time.Date(2026, time.March, 28, 2, 30, 0, 0, berlin)

	next := schedule.NextFire(previous).In(berlin)
	assert.Equal(t, 2026, next.Year(), "year")
	assert.Equal(t, time.March, next.Month(), "month")
	assert.Equal(t, 29, next.Day(), "the run was skipped rather than rolled forward")
	assert.Equal(t, 3, next.Hour(), "the run did not roll into the hour that exists")
}

func TestACalendarScheduleWithNoZoneReadsUTC(t *testing.T) {
	t.Parallel()

	tests := map[string]Schedule{
		"nil function":  Daily{Hour: 2},
		"no zone found": Daily{Hour: 2, Zone: func() *time.Location { return nil }},
	}
	for name, schedule := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			previous := time.Date(2026, time.August, 30, 0, 0, 0, 0, time.UTC)
			assert.Equal(t,
				time.Date(2026, time.August, 30, 2, 0, 0, 0, time.UTC),
				schedule.NextFire(previous), "next fire")
		})
	}
}
