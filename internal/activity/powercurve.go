package activity

import (
	"time"

	"github.com/nobbs/domestique/internal/rider"
)

// PowerBests works out the best mean power this ride held over each of the
// curve's durations.
//
// Measured power only: an estimate from the track is a different kind of number
// and a curve is compared against other riders' meters. A ride shorter than a
// duration holds no best for it, which is what leaves the long end of a curve
// empty until a long ride arrives.
func (s *RideSamples) PowerBests() rider.PowerCurve {
	curve := rider.PowerCurve{}
	if len(s.Power) == 0 {
		return curve
	}
	times := make([]time.Time, len(s.Power))
	values := make([]float64, len(s.Power))
	for index := range s.Power {
		times[index], values[index] = s.Power[index].At, s.Power[index].Value
	}
	for point, window := range rider.PowerCurveDurations() {
		if best, ok := rider.BestAverage(times, values, window); ok {
			curve.Watts[point], curve.Held[point] = best, true
		}
	}

	return curve
}
