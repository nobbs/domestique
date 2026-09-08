package rider

import (
	"math"
	"slices"
)

// StoppingMinimumRides is the fewest rides a habit is read from. A median and
// quartiles over one or two rides describe those rides, not a habit.
const StoppingMinimumRides = 5

// The floors below which a recorded activity is not riding: a paused start or a
// trip to the shops measures the errand, not the habit. This package's own
// judgement, which the ride model happens to share rather than supply.
const (
	stoppingMinimumDistanceMetres = 1000
	stoppingMinimumMovingSeconds  = 60
	secondsPerHour                = 3600
)

// StoppingRide is one recorded activity as a stopping habit reads it: the
// summary figures its device reported, not its records.
type StoppingRide struct {
	DistanceMetres float64
	MovingSeconds  float64
	ElapsedSeconds float64
}

// Stopping is how long the rider stood still per hour of moving over their own
// recent rides, as the median and the quartiles either side of it. Absent until
// enough rides carry it, which is what leaves a new rider the seeded figures.
type Stopping struct {
	MedianSecondsPerHour        float64
	LowerQuartileSecondsPerHour float64
	UpperQuartileSecondsPerHour float64
	Rides                       int
	Set                         bool
}

// MeasureStopping reads the habit off the rides that are riding at all. The
// median and quartiles are what make one forgotten stop bend the answer rather
// than set it, so no upper bound is placed on a single ride's stopping.
func MeasureStopping(rides []StoppingRide) Stopping {
	rates := make([]float64, 0, len(rides))
	for _, ride := range rides {
		if rate, ok := ride.stoppedSecondsPerMovingHour(); ok {
			rates = append(rates, rate)
		}
	}
	if len(rates) < StoppingMinimumRides {
		return Stopping{}
	}
	slices.Sort(rates)

	return Stopping{
		MedianSecondsPerHour:        quantile(rates, 0.5),
		LowerQuartileSecondsPerHour: quantile(rates, 0.25),
		UpperQuartileSecondsPerHour: quantile(rates, 0.75),
		Rides:                       len(rates),
		Set:                         true,
	}
}

// stoppedSecondsPerMovingHour is elapsed less moving, over moving. A device that
// reported more moving than elapsed contradicts itself, and one whose summary
// does not reduce to a finite rate is corrupt; both are left out.
func (r StoppingRide) stoppedSecondsPerMovingHour() (float64, bool) {
	stopped := r.ElapsedSeconds - r.MovingSeconds
	if r.DistanceMetres < stoppingMinimumDistanceMetres ||
		r.MovingSeconds < stoppingMinimumMovingSeconds ||
		stopped < 0 {
		return 0, false
	}
	rate := stopped * secondsPerHour / r.MovingSeconds
	if math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, false
	}

	return rate, true
}

// quantile interpolates between the two samples the fraction falls between,
// over a sorted, non-empty series.
func quantile(sorted []float64, fraction float64) float64 {
	position := fraction * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))

	return sorted[lower] + (sorted[upper]-sorted[lower])*(position-float64(lower))
}
