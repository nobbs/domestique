package rider

import "time"

// The shape that marks a ride as a Zwift ramp test rather than any other ride
// holding a hard minute: 20-40 minutes long, with a best minute within 1.25x of
// the best five. Recorded ramp tests sit at 1.14; a session built on one-minute
// intervals sits at 1.56 and above, and a longer training ride runs past the
// length.
//
// The shape does not try to be exact, because it does not have to be. The ratio
// bounds what a wrong answer can cost: an estimate is 75% of a best minute that
// is itself at most 1.25x the best five, so it can never exceed 94% of a rider's
// best five-minute power, and a steady ride mistaken for a ramp estimates below
// the threshold it is compared against. Since the suggestion keeps whichever
// estimate is higher, a mis-read ride loses to the twenty-minute one rather than
// inflating it.
//
// Deliberately absent: a test that the hardest minute is the ride's last. A ramp
// is ridden to failure, but the recorded ride carries the cooldown after it, so
// the peak minute of a real one ends 11 to 16 minutes before the last sample.
// That condition rejected every genuine ramp test in the corpus.
const (
	RampTestMinMovingTime = 20 * time.Minute
	RampTestMaxMovingTime = 40 * time.Minute
	rampTestPowerRatioMax = 1.25
	rampTestRatioWindow   = 5 * time.Minute
	// rampThresholdShare is the ramp test protocol's own scaling: 75% of the
	// best one-minute power, distinct from ThresholdPower's 95% of twenty.
	rampThresholdShare = 0.75
)

// RampThresholdPower estimates FTP the way a Zwift ramp test does, from one
// ride's own power series, and reports whether the ride was shaped like one.
// The recorded span stands in for moving time: a ramp test is a whole ride, and
// a rider climbing to failure takes no pause the two would differ over.
func RampThresholdPower(times []time.Time, watts []float64) (float64, bool) {
	if len(times) == 0 || len(times) != len(watts) {
		return 0, false
	}
	last := times[len(times)-1]
	if span := last.Sub(times[0]); span < RampTestMinMovingTime || span > RampTestMaxMovingTime {
		return 0, false
	}
	bestMinute, found := BestAverage(times, watts, time.Minute)
	if !found {
		return 0, false
	}
	bestFive, found := BestAverage(times, watts, rampTestRatioWindow)
	if !found || bestFive <= 0 || bestMinute/bestFive > rampTestPowerRatioMax {
		return 0, false
	}

	return bestMinute * rampThresholdShare, true
}
