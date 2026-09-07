package measure

import (
	"math"
	"time"
)

// A force-balance model of the power a ride took from the track it recorded,
// for the bicycles that carry no meter. It is a plain physics model over
// position, altitude and time; everything it produces is an estimate and is
// named as one everywhere it is stored or served.
//
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

// Estimate is one sample's estimated power. Absent where the track gave the
// model nothing to work from — the first sample of a ride or of a stretch after
// a pause, which has no step behind it to measure a speed over.
type Estimate struct {
	Watts float64
	Known bool
}

// Quality is what the estimate's own shape says about whether to trust it: a
// real ride's power is strongly autocorrelated sample to sample and moves by a
// few watts a second, and a series driven by recorder noise is neither. See
// docs/specs/measurement.md §Estimated power.
type Quality struct {
	// Autocorrelation1 is the lag-1 Pearson correlation of the estimated watts
	// with themselves shifted by one sample.
	Autocorrelation1 float64
	// MeanAbsDeltaWattsPerSecond is the mean absolute change in watts per
	// second of elapsed time between consecutive samples.
	MeanAbsDeltaWattsPerSecond float64
	// ClipBiasWatts is the mean amount the zero clamp added: clamped watts
	// minus the unclamped force·speed it would otherwise have reported.
	ClipBiasWatts float64
}

// EstimateSeries works out the power at each sample, aligned one for one with
// them, its own quality diagnostics, and whether the ride yielded any estimate
// at all.
//
// totalMassKG is the rider and everything they are carrying, the bicycle
// included: gravity acts on the whole of it.
//
// See docs/specs/measurement.md §Estimated power.
func EstimateSeries(samples []Sample, totalMassKG float64) ([]Estimate, Quality, bool) {
	if totalMassKG <= 0 || len(samples) < 2 {
		return nil, Quality{}, false
	}
	estimates := make([]Estimate, len(samples))
	times := make([]time.Time, len(samples))
	for index, sample := range samples {
		times[index] = sample.At
	}
	bounds := Stretches(times, DefaultMaxGap)
	known := false
	var (
		deltaSum, deltaCount   float64
		clipBiasSum, clipCount float64
		series, shifted        []float64
	)
	for index := 1; index < len(samples); index++ {
		// The step to the sample before is what marks a pause; the window the
		// speed and grade are then measured over never crosses one.
		step := samples[index].At.Sub(samples[index-1].At)
		if step <= 0 || step > DefaultMaxGap {
			continue
		}
		low, high := centredWindow(samples, index, bounds)
		span := samples[high].At.Sub(samples[low].At).Seconds()
		run := samples[high].DistanceMetres - samples[low].DistanceMetres
		if span <= 0 || run < 0 {
			continue
		}
		clamped, clipBias := watts(run/span, slope(samples[low], samples[high]), totalMassKG)
		estimates[index] = Estimate{Watts: clamped, Known: true}
		known = true
		clipBiasSum += clipBias
		clipCount++
		if estimates[index-1].Known {
			delta := clamped - estimates[index-1].Watts
			deltaSum += math.Abs(delta) / step.Seconds()
			deltaCount++
			series = append(series, estimates[index-1].Watts)
			shifted = append(shifted, clamped)
		}
	}

	quality := Quality{ClipBiasWatts: safeMean(clipBiasSum, clipCount)}
	if deltaCount > 0 {
		quality.MeanAbsDeltaWattsPerSecond = deltaSum / deltaCount
	}
	quality.Autocorrelation1 = correlation(series, shifted)

	return estimates, quality, known
}

// safeMean is total / count, or zero for a zero count.
func safeMean(total, count float64) float64 {
	if count == 0 {
		return 0
	}

	return total / count
}

// correlation is the Pearson correlation between two equal-length series, or
// zero for fewer than three pairs or a series with no variance to correlate.
func correlation(a, b []float64) float64 {
	n := float64(len(a))
	if n < 3 {
		return 0
	}
	var meanA, meanB float64
	for index := range a {
		meanA += a[index]
		meanB += b[index]
	}
	meanA /= n
	meanB /= n
	var cov, varA, varB float64
	for index := range a {
		da, db := a[index]-meanA, b[index]-meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}
	if varA <= 0 || varB <= 0 {
		return 0
	}

	return cov / math.Sqrt(varA*varB)
}

// watts is the model itself: what it costs to climb the grade, roll along it
// and push the air aside, all at once. It reports the watts clamped at zero —
// a rider freewheeling down a hill is putting nothing in, and the model has no
// way to say they are taking something out — alongside how much the clamp
// added, zero where it did not fire.
func watts(speed, grade, totalMassKG float64) (clamped, clipBias float64) {
	weight := totalMassKG * gravity
	force := weight*grade + weight*rollingResistance + 0.5*airDensity*dragArea*speed*speed
	unclamped := force * speed
	if unclamped > 0 {
		return unclamped, 0
	}

	return 0, -unclamped
}

// centredWindow is the span of samples around one sample that covers
// windowMetres of distance, or as much of it as that stretch of recording
// holds. Both bounds are inclusive.
func centredWindow(samples []Sample, index int, bounds []Stretch) (low, high int) {
	within := bounds[index]
	// A stretch shorter than the window has one answer for every sample in it,
	// so it is measured end to end rather than walked outward from each. A long
	// stationary stretch is exactly that case, and walking it per sample would
	// cost the square of its length.
	if run := samples[within.Past-1].DistanceMetres - samples[within.First].DistanceMetres; run < windowMetres {
		return within.First, within.Past - 1
	}
	// The early return above guarantees the full stretch already reaches
	// windowMetres, so expanding to it always satisfies the loop below.
	low, high = index, index
	for samples[high].DistanceMetres-samples[low].DistanceMetres < windowMetres {
		if low > within.First {
			low--
		}
		if high < within.Past-1 {
			high++
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

// MeanEstimate is the mean of the estimates that were worked out, and whether
// there were any. It is what a ride shows beside its other per-ride numbers.
//
// See docs/specs/measurement.md §Estimated power.
func MeanEstimate(estimates []Estimate) (float64, bool) {
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
