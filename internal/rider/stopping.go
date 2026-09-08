package rider

import (
	"math"
	"slices"
)

// StoppingMinimumRides is the fewest rides a habit is read from. A median and
// quartiles over one or two rides describe those rides, not a habit.
const StoppingMinimumRides = 5

// The floors below which a recorded activity is not riding, matching the ones
// the ride model calibrates over.
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

// stoppedSecondsPerMovingHour is elapsed less moving, over moving. A device
// that reported more moving than elapsed contradicts itself, so it is left out.
func (r StoppingRide) stoppedSecondsPerMovingHour() (float64, bool) {
	stopped := r.ElapsedSeconds - r.MovingSeconds
	if r.DistanceMetres < stoppingMinimumDistanceMetres ||
		r.MovingSeconds < stoppingMinimumMovingSeconds ||
		stopped < 0 || math.IsNaN(stopped) {
		return 0, false
	}

	return stopped * secondsPerHour / r.MovingSeconds, true
}

// quantile interpolates between the two samples the fraction falls between,
// over a sorted, non-empty series.
func quantile(sorted []float64, fraction float64) float64 {
	position := fraction * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))

	return sorted[lower] + (sorted[upper]-sorted[lower])*(position-float64(lower))
}
