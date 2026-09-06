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
}

// ThresholdPower scales a best twenty-minute average to the hour power it
// implies.
func ThresholdPower(bestTwentyMinuteWatts float64) float64 {
	return bestTwentyMinuteWatts * thresholdPowerShare
}
