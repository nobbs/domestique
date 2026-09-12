package rider

import (
	"time"

	"github.com/nobbs/domestique/internal/measure"
)

// The shape that marks a ride as a Zwift ramp test rather than any other ride
// holding a hard minute: 20-40 minutes of unbroken recording, with a best
// minute within 1.25x of the best five. Recorded ramp tests sit at 1.14; a
// session built on one-minute intervals sits at 1.56 and above, and a longer
// training ride runs past the length.
//
// The shape is loose, and deliberately does not decide anything by itself. The
// ratio bounds what one ride can claim — an estimate is 75% of a minute that is
// itself at most 1.25x the best five, so it never exceeds 94% of the five
// minutes it was read over — but that bound is on the ride and not on the
// rider. An easy ride clears it: 16 minutes at 50 W, 4 at 100 and a closing
// minute at 125 is a ratio of 1.19 and offers 94 W to a rider who never held
// 60. Whether a reading means anything is therefore settled by the rider's
// other rides, not here; see thresholdPowerFrom in internal/sqlite.
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
//
// The recorded span stands in for moving time, which holds only while the
// recording is unbroken: a pause would let a few valid minutes and one late
// sample present themselves as a ride of the right length. A step longer than
// the recording gap ends a stretch (§Recording gaps), and a ramp is one
// stretch, so any such step disqualifies the ride rather than dividing it.
func RampThresholdPower(times []time.Time, watts []float64) (float64, bool) {
	if len(times) == 0 || len(times) != len(watts) {
		return 0, false
	}
	span := times[len(times)-1].Sub(times[0])
	if span < RampTestMinMovingTime || span > RampTestMaxMovingTime {
		return 0, false
	}
	for index := 1; index < len(times); index++ {
		if times[index].Sub(times[index-1]) > measure.DefaultMaxGap {
			return 0, false
		}
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
