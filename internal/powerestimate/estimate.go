// Package powerestimate works out the power a ride took from the track it
// recorded, for the bicycles that carry no meter. It is a plain physics model
// over position, altitude and time; everything it produces is an estimate and
// is named as one everywhere it is stored or served.
package powerestimate

import "time"

// The constants a road bicycle is modelled at. They are fixed rather than
// configurable: a rider cannot measure their own Crr or CdA, and a number they
// would have to guess is worse than one this model states plainly.
//
// CdA is a rider on the hoods, Crr is a good clincher on tarmac, and the air is
// taken at sea level and fifteen degrees. Wind is ignored entirely — the model
// has no idea which way the rider was pointing relative to it.
const (
	gravity           = 9.80665 // m/s²
	rollingResistance = 0.005
	dragArea          = 0.32  // m², CdA
	airDensity        = 1.225 // kg/m³
	// Drivetrain loss is not modelled. It is a couple of per cent on a figure
	// already labelled an estimate, and one more constant to defend.
)

// windowMetres is the distance both the grade and the speed are measured over.
// Sample to sample, altitude is barometric noise as much as hill and distance
// is GPS jitter as much as travel; thirty metres is short enough to follow a
// real ramp and long enough not to invent one.
//
// Measuring speed over the same window is what keeps the zero clamp in watts
// honest. Fed raw one-second deltas the clamp fires on half the noise and keeps
// the other half, which on a real ride inflated the mean fourfold.
const windowMetres = 30

// maxSampleGap is the longest step the model will cross. Beyond it the recorder
// had stopped, and a speed worked out across the pause is not a speed.
const maxSampleGap = 10 * time.Second

// Sample is one recorded moment of the track this model reads: when, how far
// along, and how high. A record missing any of the three is not a sample.
type Sample struct {
	At             time.Time
	DistanceMetres float64
	AltitudeMetres float64
}

// Estimate is one sample's estimated power. Absent where the track gave the
// model nothing to work from — the first sample of a ride or of a stretch after
// a pause, which has no step behind it to measure a speed over.
type Estimate struct {
	Watts float64
	Known bool
}

// Series works out the power at each sample, aligned one for one with them, and
// reports whether the ride yielded any estimate at all.
//
// totalMassKG is the rider and everything they are carrying, the bicycle
// included: gravity acts on the whole of it.
func Series(samples []Sample, totalMassKG float64) ([]Estimate, bool) {
	if totalMassKG <= 0 || len(samples) < 2 {
		return nil, false
	}
	estimates := make([]Estimate, len(samples))
	bounds := unbrokenStretches(samples)
	known := false
	for index := 1; index < len(samples); index++ {
		// The step to the sample before is what marks a pause; the window the
		// speed and grade are then measured over never crosses one.
		if step := samples[index].At.Sub(samples[index-1].At); step <= 0 || step > maxSampleGap {
			continue
		}
		low, high := windowAt(samples, index, bounds)
		span := samples[high].At.Sub(samples[low].At).Seconds()
		run := samples[high].DistanceMetres - samples[low].DistanceMetres
		if span <= 0 || run < 0 {
			continue
		}
		estimates[index] = Estimate{
			Watts: watts(run/span, slope(samples[low], samples[high]), totalMassKG),
			Known: true,
		}
		known = true
	}

	return estimates, known
}

// watts is the model itself: what it costs to climb the grade, roll along it
// and push the air aside, all at once.
//
// Clamped at zero, never negative: a rider freewheeling down a hill is putting
// nothing in, and the model has no way to say they are taking something out.
func watts(speed, grade, totalMassKG float64) float64 {
	weight := totalMassKG * gravity
	force := weight*grade + weight*rollingResistance + 0.5*airDensity*dragArea*speed*speed
	if power := force * speed; power > 0 {
		return power
	}

	return 0
}

// stretch is the half-open range of samples recorded without a pause in them.
type stretch struct {
	first int
	past  int
}

// unbrokenStretches marks, for each sample, the stretch of recording it belongs
// to. Neither a grade nor a speed may be measured across a pause: the altitude
// either side of one is minutes of barometric drift apart, and the distance
// between them was covered over a gap that is not riding time.
func unbrokenStretches(samples []Sample) []stretch {
	bounds := make([]stretch, len(samples))
	for first := 0; first < len(samples); {
		past := first + 1
		for past < len(samples) && samples[past].At.Sub(samples[past-1].At) <= maxSampleGap {
			past++
		}
		for index := first; index < past; index++ {
			bounds[index] = stretch{first: first, past: past}
		}
		first = past
	}

	return bounds
}

// windowAt is the span of samples around one sample that covers windowMetres of
// distance, or as much of it as that stretch of recording holds. Both bounds
// are inclusive.
func windowAt(samples []Sample, index int, bounds []stretch) (low, high int) {
	within := bounds[index]
	// A stretch shorter than the window has one answer for every sample in it,
	// so it is measured end to end rather than walked outward from each. A long
	// stationary stretch is exactly that case, and walking it per sample would
	// cost the square of its length.
	if run := samples[within.past-1].DistanceMetres - samples[within.first].DistanceMetres; run < windowMetres {
		return within.first, within.past - 1
	}
	low, high = index, index
	for samples[high].DistanceMetres-samples[low].DistanceMetres < windowMetres {
		moved := false
		if low > within.first {
			low--
			moved = true
		}
		if high < within.past-1 {
			high++
			moved = true
		}
		if !moved {
			break
		}
	}

	return low, high
}

// slope is the rise between two samples over the distance between them, and
// zero where they cover no distance at all: a rider who has not moved is on no
// gradient this model can name.
func slope(low, high Sample) float64 {
	run := high.DistanceMetres - low.DistanceMetres
	if run <= 0 {
		return 0
	}

	return (high.AltitudeMetres - low.AltitudeMetres) / run
}

// Average is the mean of the estimates that were worked out, and whether there
// were any. It is what a ride shows beside its other per-ride numbers.
func Average(estimates []Estimate) (float64, bool) {
	total, count := 0.0, 0
	for _, estimate := range estimates {
		if estimate.Known {
			total += estimate.Watts
			count++
		}
	}
	if count == 0 {
		return 0, false
	}

	return total / float64(count), true
}
