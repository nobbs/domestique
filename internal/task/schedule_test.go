package task

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

// A gap an operator has emptied is a schedule that is not due again, rather
// than one due immediately and forever.
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

// A run that outlasted its gap starts again at once. Queueing the gaps it
// missed would make one slow run cost several immediate ones.
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
