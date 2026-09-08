// Package rider holds what a service knows about one rider's own body and
// equipment, and what its stored rides suggest those numbers are.
package rider

import "time"

// Profile is the handful of numbers every derived training metric needs about
// one rider. Every field is optional: a rider who has entered nothing has an
// empty profile, not a profile of zeroes, and a zero heart rate or mass is a
// value no downstream calculation could use anyway.
type Profile struct {
	// MaxHeartRateBPM and RestingHeartRateBPM bound the heart-rate reserve;
	// ThresholdHeartRateBPM is the lactate threshold rate zones are cut at.
	MaxHeartRateBPM       Value
	RestingHeartRateBPM   Value
	ThresholdHeartRateBPM Value
	// FunctionalThresholdPowerWatts is the hour power the rider holds.
	FunctionalThresholdPowerWatts Value
	// RiderMassKG is the rider alone; BikeMassKG the bicycle and everything on
	// it. A climb needs their sum, so both are kept rather than one total.
	RiderMassKG Value
	BikeMassKG  Value
}

// Value is one profile number, which is either set or absent.
type Value struct {
	Number float64
	Set    bool
}

// Set is the value a rider entered.
func Set(number float64) Value {
	return Value{Number: number, Set: true}
}

// Pointer renders the value the way a nullable column and a JSON field both
// want it: nil when the rider has entered nothing.
func (v Value) Pointer() *float64 {
	if !v.Set {
		return nil
	}
	number := v.Number

	return &number
}

// FromPointer reads a nullable column or an omitted JSON field back.
func FromPointer(number *float64) Value {
	if number == nil {
		return Value{}
	}

	return Set(*number)
}

// The windows a suggestion is the best effort over, and the share of a best
// twenty-minute power that is taken for a threshold hour power — the
// conventional 95%, the same figure a ramp test is scaled by.
const (
	MaxHeartRateWindow   = time.Minute
	ThresholdPowerWindow = 20 * time.Minute
	thresholdPowerShare  = 0.95
)

// SuggestionWindow is how far back a suggestion reads. Fitness moves, so a best
// effort from years ago is not a suggestion about this rider now — and it also
// keeps the scan off every sample the service has ever stored.
const SuggestionWindow = 90 * 24 * time.Hour

// Suggestions are what the rider's recent rides say their numbers could be,
// offered beside the fields and never stored. A sensor the rides do not carry
// yields no suggestion rather than a zero.
type Suggestions struct {
	MaxHeartRateBPM               Value
	FunctionalThresholdPowerWatts Value
	// Stopping is the rider's own stopping habit, which is a distribution
	// rather than a best effort and so is not a Value.
	Stopping Stopping
}

// ThresholdPower scales a best twenty-minute average to the hour power it
// implies.
func ThresholdPower(bestTwentyMinuteWatts float64) float64 {
	return bestTwentyMinuteWatts * thresholdPowerShare
}

// PowerCurvePoints is how many durations the power-duration curve is read at.
const PowerCurvePoints = 6

// PowerCurveDurations are those durations, shortest first: a neuromuscular
// sprint, a standing start, a minute, a hard five, the threshold window the FTP
// suggestion is taken from, and an hour. Returned rather than held, so no
// caller can reorder the set a stored row is keyed by.
func PowerCurveDurations() [PowerCurvePoints]time.Duration {
	return [PowerCurvePoints]time.Duration{
		5 * time.Second,
		30 * time.Second,
		time.Minute,
		5 * time.Minute,
		ThresholdPowerWindow,
		time.Hour,
	}
}

// ThresholdPowerPoint is where ThresholdPowerWindow sits in the curve. The
// suggestion is worked out from the samples rather than read off the curve, for
// the reason measurement.md gives; this pins both to the one window constant so
// they cannot come to describe different twenty minutes.
const ThresholdPowerPoint = 4

// PowerCurve is the best mean power held over each of those durations, over
// whatever rides it was folded from. A duration no ride was long enough for is
// absent rather than nought.
type PowerCurve struct {
	Watts [PowerCurvePoints]float64
	Held  [PowerCurvePoints]bool
}

// Any reports whether the curve holds a single point.
func (c *PowerCurve) Any() bool {
	for _, held := range c.Held {
		if held {
			return true
		}
	}

	return false
}
