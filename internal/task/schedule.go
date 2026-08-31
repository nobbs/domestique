package task

import "time"

// Schedule decides when a task runs.
type Schedule interface {
	// NextFire reports when a task last due at previous is due again, or the
	// zero time when it is not due again at all.
	NextFire(previous time.Time) time.Time
}

// firesAtStart marks a schedule whose first run is the moment it starts rather
// than a time of its own, like a fixed gap. The method is unexported, so a
// schedule from outside this package defaults to the calendar behavior — the
// safer of the two to be wrong about.
type firesAtStart interface{ firesAtStart() }

// Every is a fixed gap between runs. The gap is read again before each wait, so
// an operator's edit takes effect at the next gap rather than the next restart.
type Every func() time.Duration

// firesAtStart marks Every as running as soon as its initial delay is out.
func (Every) firesAtStart() {}

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
// missed.
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

// Zone is where local time is read, supplied as a function so an operator's
// edit reaches the next wait rather than the next restart. A nil zone, or one
// that reports nothing, is UTC.
type Zone func() *time.Location

func (z Zone) location() *time.Location {
	if z == nil {
		return time.UTC
	}
	if location := z(); location != nil {
		return location
	}

	return time.UTC
}

// Daily fires once a day at a wall-clock time, in the zone the service reads
// local time in.
type Daily struct {
	Zone   Zone
	Hour   int
	Minute int
}

// NextFire is the first occurrence of the wall-clock time strictly after
// previous.
func (d Daily) NextFire(previous time.Time) time.Time {
	return nextWallClock(previous, d.Zone, d.Hour, d.Minute, func(time.Time) bool { return true })
}

// Weekly fires once a week, on one weekday at a wall-clock time.
type Weekly struct {
	Zone    Zone
	Weekday time.Weekday
	Hour    int
	Minute  int
}

// NextFire is the first occurrence of the weekday and wall-clock time strictly
// after previous.
func (w Weekly) NextFire(previous time.Time) time.Time {
	return nextWallClock(previous, w.Zone, w.Hour, w.Minute, func(candidate time.Time) bool {
		return candidate.Weekday() == w.Weekday
	})
}

// daysSearched bounds the walk forward. A week and a day covers every weekday
// from any starting point, with a day to spare for one a zone shift moved.
const daysSearched = 8

// nextWallClock walks day by day from previous for the first day the schedule
// accepts whose wall-clock time has not already passed.
//
// The walk is over calendar days rather than fixed 24-hour steps, because those
// are not the same thing twice a year. Where the wall clock skips the named
// hour, time.Date rolls forward into the hour that does exist rather than
// refusing, so a spring-forward run happens an hour late instead of being
// skipped until autumn.
func nextWallClock(previous time.Time, zone Zone, hour, minute int, accepts func(time.Time) bool) time.Time {
	location := zone.location()
	local := previous.In(location)
	for day := range daysSearched {
		candidate := time.Date(
			local.Year(), local.Month(), local.Day()+day, hour, minute, 0, 0, location,
		)
		if candidate.After(previous) && accepts(candidate) {
			return candidate
		}
	}

	return time.Time{}
}
