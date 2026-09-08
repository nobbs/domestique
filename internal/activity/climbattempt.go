package activity

import (
	"time"

	"github.com/nobbs/domestique/internal/measure"
)

// The climb detector's own two constants: a gradient measured back over a
// hundred metres, and three percent to count as climbing rather than merely
// uphill. See docs/specs/measurement.md §Sustained climbs.
const (
	ClimbWindowMetres       = 100
	ClimbMinGradientPercent = 3
)

// climbCoverageToleranceMetres is how much of a climb's ends a ride may be
// missing and still be said to have ridden it. One window: a ride that joined
// the road a few paces up the climb rode the climb, and one that stopped a
// kilometre short did not.
const climbCoverageToleranceMetres = ClimbWindowMetres

// ClimbAttempt is one ride over one of its route's sustained climbs: how long
// it took, and what the rider's sensors said while it lasted. What the climb
// *is* — where it runs, how much it rises, how steep it averages — belongs to
// the route and is not repeated here, so no attempt can disagree with the climb
// it is an attempt at.
//
// Measured and estimated power are kept apart and never summed or ranked
// together: an estimate is a different kind of number.
type ClimbAttempt struct {
	// ClimbIndex is which of the route's climbs this is, in the order Climbs
	// reports them, which is the order they are ridden.
	ClimbIndex int
	Seconds    float64
	// HeartRateBPM and PowerWatts are means over the samples inside the climb.
	HeartRateBPM        float64
	PowerWatts          float64
	EstimatedPowerWatts float64
	HasHeartRate        bool
	HasPower            bool
	HasEstimatedPower   bool
}

// StoredClimbAttempt is one attempt read back, with the ride that made it.
type StoredClimbAttempt struct {
	RiddenAt time.Time
	ClimbAttempt
	WorkoutID int64
}

// RouteClimbs finds the sustained climbs of one library route, in ride order.
// Empty for a route whose stored geometry carries no elevation, which is a
// route nothing can be timed over rather than a route with no climbs.
func RouteClimbs(line []measure.Coordinate, elevations []float64) []measure.Climb {
	if len(line) != len(elevations) || len(line) < 2 {
		return nil
	}
	distances := make([]float64, len(line))
	for index := 1; index < len(line); index++ {
		distances[index] = distances[index-1] + measure.HaversineMetres(line[index-1], line[index])
	}
	profile, ok := measure.ProfileOf(distances, elevations)
	if !ok {
		return nil
	}

	return measure.Climbs(profile, ClimbWindowMetres, ClimbMinGradientPercent)
}

// ClimbAttempts times one ride over each of its route's climbs.
//
// A ride is timed only where it ran the way the route is stored: ridden the
// other way round, a route's climbs are its descents, and a descent timed as a
// climb is not a slower attempt but a different thing entirely.
//
// track and series are the same ride's positioned samples, indexed 1:1.
func ClimbAttempts(
	climbs []measure.Climb,
	line []measure.Coordinate,
	track []TrackPoint,
	series []SampleRow,
	direction Direction,
) []ClimbAttempt {
	if direction != DirectionForward || len(climbs) == 0 || len(track) < 2 || len(series) != len(track) {
		return nil
	}
	along := alongRoute(line, track)
	attempts := make([]ClimbAttempt, 0, len(climbs))
	for index := range climbs {
		if attempt, ok := attemptOver(&climbs[index], along, track, series); ok {
			attempt.ClimbIndex = index
			attempts = append(attempts, attempt)
		}
	}

	return attempts
}

// alongRoute is where each of the ride's samples fell along the route, and
// whether it fell on the route at all. A sample off the corridor has no place
// on the route and takes no part in any climb.
func alongRoute(line []measure.Coordinate, track []TrackPoint) []measure.SnapHit {
	index := measure.NewSnapIndex([][]measure.Coordinate{line}, corridorMetres)
	hits := make([]measure.SnapHit, len(track))
	for point := range track {
		hit, found := index.Nearest(measure.Coordinate{
			Latitude: track[point].Latitude, Longitude: track[point].Longitude,
		})
		if found {
			hits[point] = hit
		} else {
			// Marked as off the route by a distance no corridor admits.
			hits[point] = measure.SnapHit{AlongMetres: -1}
		}
	}

	return hits
}

// attemptOver times the ride's first run through one climb. The first, because
// a ride that passes a climb twice rode it twice, and the fastest of two
// passes is a question this service does not yet answer.
//
// A run must reach both ends of the climb within the tolerance: a ride that
// turned back half way up did not ride it.
func attemptOver(
	climb *measure.Climb, along []measure.SnapHit, track []TrackPoint, series []SampleRow,
) (ClimbAttempt, bool) {
	first, last := -1, -1
	for index := range along {
		inside := along[index].AlongMetres >= climb.StartMetres &&
			along[index].AlongMetres <= climb.EndMetres
		if inside {
			if first < 0 {
				first = index
			}
			last = index

			continue
		}
		// The first run and no more: once the ride has left the climb, a later
		// pass is a second attempt rather than a continuation of this one.
		if first >= 0 {
			break
		}
	}
	if first < 0 || last <= first {
		return ClimbAttempt{}, false
	}
	if along[first].AlongMetres > climb.StartMetres+climbCoverageToleranceMetres ||
		along[last].AlongMetres < climb.EndMetres-climbCoverageToleranceMetres {
		return ClimbAttempt{}, false
	}
	seconds := track[last].Time.Sub(track[first].Time).Seconds()
	if seconds <= 0 {
		return ClimbAttempt{}, false
	}
	attempt := ClimbAttempt{Seconds: seconds}
	attempt.HeartRateBPM, attempt.HasHeartRate = meanReading(series[first:last+1], heartRateOf)
	attempt.PowerWatts, attempt.HasPower = meanReading(series[first:last+1], powerOf)
	attempt.EstimatedPowerWatts, attempt.HasEstimatedPower = meanEstimated(track[first : last+1])

	return attempt, true
}

func heartRateOf(row *SampleRow) Reading { return row.HeartRateBPM }
func powerOf(row *SampleRow) Reading     { return row.PowerWatts }

// meanReading averages one series over the samples that carried it.
func meanReading(rows []SampleRow, of func(*SampleRow) Reading) (float64, bool) {
	total, counted := 0.0, 0
	for index := range rows {
		if reading := of(&rows[index]); reading.Known {
			total += reading.Value
			counted++
		}
	}
	if counted == 0 {
		return 0, false
	}

	return total / float64(counted), true
}

// meanEstimated averages the estimate over the samples that carry one.
func meanEstimated(points []TrackPoint) (float64, bool) {
	total, counted := 0.0, 0
	for index := range points {
		if points[index].HasEstimatedPower {
			total += points[index].EstimatedPowerWatts
			counted++
		}
	}
	if counted == 0 {
		return 0, false
	}

	return total / float64(counted), true
}
