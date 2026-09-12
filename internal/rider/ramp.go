package rider

import "time"

// The shape that marks a ride as a Zwift ramp test rather than an interval
// session that merely contains one hard minute. Calibrated against recorded
// ramp tests, which run 20-40 minutes moving and hold a best minute within
// 1.25x of their best five, sitting at 1.14 in practice; an interval session
// with a hard minute in it sits at 1.56 and above, so the gap is wide.
const (
	RampTestMinMovingTime = 20 * time.Minute
	RampTestMaxMovingTime = 40 * time.Minute
	rampTestPowerRatioMax = 1.25
	// rampThresholdShare is the ramp test protocol's own scaling: 75% of the
	// best one-minute power, distinct from ThresholdPower's 95% of twenty.
	rampThresholdShare = 0.75
)

// RampThresholdPower estimates FTP the way a Zwift ramp test does, for a ride
// shaped like one, and reports whether it was.
func RampThresholdPower(movingTime time.Duration, bestOneMinuteWatts, bestFiveMinuteWatts float64) (float64, bool) {
	if movingTime < RampTestMinMovingTime || movingTime > RampTestMaxMovingTime {
		return 0, false
	}
	if bestFiveMinuteWatts <= 0 || bestOneMinuteWatts/bestFiveMinuteWatts > rampTestPowerRatioMax {
		return 0, false
	}

	return bestOneMinuteWatts * rampThresholdShare, true
}
