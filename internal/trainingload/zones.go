// Package trainingload works out what one ride's recorded samples and one
// rider's profile say about how hard that ride was. It reads samples and
// numbers only: no position, no altitude, no provider, so a ride recorded on a
// trainer derives exactly as one ridden outdoors.
package trainingload

import (
	"math"

	"github.com/nobbs/domestique/internal/measure"
)

// Zones is how long a ride was spent in each of five heart-rate zones, in
// seconds, easiest first.
type Zones [5]float64

// Total is the time the zones account for.
func (z Zones) Total() float64 {
	total := 0.0
	for _, seconds := range z {
		total += seconds
	}

	return total
}

// Bounds are the four heart rates that separate five zones, ascending. A
// sample below the first is zone one; one at or above the last is zone five.
type Bounds [4]float64

// The two zone schemes, as shares of the rate they are cut from.
//
// From the lactate threshold, the usual five-zone cut: below 85% of it, then
// 85, 90, 95 and 100. From the maximum instead, the classic percentage-of-max
// cut at 60, 70, 80 and 90. The threshold scheme is preferred where the rider
// has entered one, because a threshold is measured and a maximum is often
// guessed.
var (
	thresholdShares = [4]float64{0.85, 0.90, 0.95, 1.00} //nolint:gochecknoglobals // the scheme itself, read-only.
	maximumShares   = [4]float64{0.60, 0.70, 0.80, 0.90} //nolint:gochecknoglobals // the scheme itself, read-only.
)

// BoundsFrom returns the zone bounds for a rider, and whether the profile said
// enough to cut any: the threshold where there is one, the maximum otherwise.
func BoundsFrom(thresholdHeartRate, maxHeartRate float64) (Bounds, bool) {
	shares, rate := maximumShares, maxHeartRate
	if thresholdHeartRate > 0 {
		shares, rate = thresholdShares, thresholdHeartRate
	}
	if rate <= 0 {
		return Bounds{}, false
	}
	bounds := Bounds{}
	for index, share := range shares {
		bounds[index] = rate * share
	}

	return bounds, true
}

// TimeInZones sums how long the ride held each zone. Each sample counts for as
// long as it stands, up to measure.DefaultMaxGap: a recorder that paused must
// not book the whole pause to whichever zone it stopped in, and a dropout the
// strap wrote as a run of noughts must not be bridged into either zone it
// falls between.
func TimeInZones(samples []Sample, bounds Bounds) Zones {
	zones := Zones{}
	for _, run := range heartRateRuns(samples) {
		measure.ForEachHeld(run, measure.DefaultMaxGap, func(value, seconds float64) {
			zones[zoneOf(value, bounds)] += seconds
		})
	}

	return zones
}

// zoneOf places one heart rate, easiest zone first.
func zoneOf(heartRate float64, bounds Bounds) int {
	zone := 0
	for _, bound := range bounds {
		if heartRate < bound {
			break
		}
		zone++
	}

	return zone
}

// Sample is one recorded moment of whichever sensor is being read.
type Sample = measure.Reading

// round4 is the fourth root, which normalized power ends on.
func round4(value float64) float64 {
	return math.Pow(value, 0.25)
}
