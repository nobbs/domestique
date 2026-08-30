package task

import "time"

// Schedule decides when a task runs.
type Schedule interface {
	// NextFire reports when a task last due at previous is due again, or the
	// zero time when it is not due again at all.
	NextFire(previous time.Time) time.Time
}

// Every is a fixed gap between runs. The gap is read again before each wait, so
// an operator's edit takes effect at the next gap rather than the next restart.
type Every func() time.Duration

// NextFire returns previous plus the gap, or the zero time when the gap is not
// a positive duration.
func (e Every) NextFire(previous time.Time) time.Time {
	gap := e()
	if gap <= 0 {
		return time.Time{}
	}

	return previous.Add(gap)
}

// nextDue is when a run last due at previous should next start. A gap that has
// already elapsed starts the next run at once rather than queueing the ones it
// missed, and the cadence carries on from there.
func nextDue(schedule Schedule, previous, now time.Time) (time.Time, bool) {
	due := schedule.NextFire(previous)
	if due.IsZero() {
		return time.Time{}, false
	}
	if due.Before(now) {
		return now, true
	}

	return due, true
}
