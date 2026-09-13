package measure

import (
	"math"
	"time"
)

// EstimateSeries is a force-balance model of the power a ride took from the
// track it recorded, for the bicycles that carry no meter: at each sample,
// what it costs to climb the grade, roll along it, push the air aside and
// return to speed, divided by the drivetrain that carries it to the road. See
// Martin et al., "Validation of a Mathematical Model for Road Cycling Power"
// (1998), and docs/references/power-estimation-handover.md for the
// coefficient table.
//
// Everything it produces is an estimate and is named as one everywhere it is
// stored or served. Air density is worked out per sample from altitude and
// temperature (see airDensity below); the track carries no notion of which
// way the rider was pointing relative to the wind, so the model does not
// model it.
const (
	gravity              = 9.80665 // m/s²
	rollingResistance    = 0.005
	dragArea             = 0.36 // m², CdA
	drivetrainEfficiency = 0.977
	// rotationalMassKG is the wheels' rotational inertia written as an
	// equivalent linear mass. Only the inertial term carries it: gravity and
	// rolling resistance act on the mass that is really there.
	rotationalMassKG = 1.5
)

// accelerationBaseline is the time the inertial term differentiates the window
// speed over. Differentiated against the sample before it instead, the term
// reads as recorder noise rather than as a rider: the handover smooths speed
// over eleven samples before differentiating for the same reason (§4).
const accelerationBaseline = 10 * time.Second

// defaultTemperatureCelsius is what a sample with no thermometer is assumed to
// have been ridden at.
const defaultTemperatureCelsius = 15

// airDensity is the density of air at altitudeMetres and temperatureCelsius,
// via the international barometric formula for pressure and the ideal gas law.
// See docs/references/power-estimation-handover.md §2.
func airDensity(altitudeMetres, temperatureCelsius float64) float64 {
	pressure := 101325 * math.Pow(1-2.25577e-5*altitudeMetres, 5.25588)

	return pressure / (287.058 * (temperatureCelsius + 273.15))
}

// The window both the grade and the speed are measured over is derived per
// ride, not fixed: a coarser altimeter needs more distance to tell a real
// ramp from its own quantisation noise. targetGradePrecision is a fifth of a
// per cent of grade, the handover's own target (§3); minWindowMetres is the
// floor a coarse-enough altimeter cannot go under; maxWindowMetres is the
// handover's own choice of a practical ceiling.
//
// Measuring speed over the same window is what keeps the zero clamp in watts
// honest. Fed raw one-second deltas the clamp fires on half the noise and keeps
// the other half, which on a real ride inflated the mean fourfold.
const (
	targetGradePrecision = 0.002
	minWindowMetres      = 30
	maxWindowMetres      = 300
)

// gradeWindowMetres is the distance the grade and speed are measured over for
// one ride: window ≥ resolution / precision, with 0.2 m of barometric
// resolution and a target precision of 0.002 giving 100 m. See
// docs/specs/measurement.md §Gradient.
//
// The altimeter's resolution is taken as the smallest positive altitude step
// between consecutive samples whose clock advanced by no more than a gap and
// whose distance did not run backward — the same steps the series itself is
// measured over, so a pause's drift, a clock that did not move, and an
// odometer reset are no reading of it — rounded to a hundredth of a metre so
// a floating-point 0.19999 reads as the 0.2 it is. A ride with no positive
// step at all, dead flat or a single sample, takes minWindowMetres.
func gradeWindowMetres(samples []Sample) float64 {
	quantum := math.Inf(1)
	for index := 1; index < len(samples); index++ {
		if held := samples[index].At.Sub(samples[index-1].At); held <= 0 || held > DefaultMaxGap {
			continue
		}
		if samples[index].DistanceMetres < samples[index-1].DistanceMetres {
			continue
		}
		if step := samples[index].AltitudeMetres - samples[index-1].AltitudeMetres; step > 0 && step < quantum {
			quantum = step
		}
	}
	if math.IsInf(quantum, 1) {
		return minWindowMetres
	}
	quantum = math.Round(quantum*100) / 100

	return min(max(quantum/targetGradePrecision, minWindowMetres), maxWindowMetres)
}

// Coefficients are the two numbers that describe the bicycle: its drag area
// and its tyres' rolling resistance. Everything else in the model is measured
// from the track, is the rider's own mass, or is a constant of nature.
type Coefficients struct {
	DragArea          float64 // m², CdA
	RollingResistance float64 // Crr
}

// DefaultCoefficients is the road bicycle every estimate is worked out at
// until a rider's own bicycle numbers replace it.
func DefaultCoefficients() Coefficients {
	return Coefficients{DragArea: dragArea, RollingResistance: rollingResistance}
}

// Valid reports whether these coefficients could have come from a real
// bicycle, rather than from a profile value gone astray.
func (c Coefficients) Valid() bool {
	return c.DragArea > 0 && c.DragArea < 2 && c.RollingResistance > 0 && c.RollingResistance < 0.1
}

// Estimate is one sample's estimated power. Absent where the track gave the
// model nothing to work from — the first sample of a ride or of a stretch after
// a pause, which has no step behind it to measure a speed over.
type Estimate struct {
	Watts float64
	Known bool
}

// EstimateSeries works out the power at each sample, aligned one for one with
// them, and whether the ride yielded any estimate at all.
//
// totalMassKG is the rider and everything they are carrying, the bicycle
// included: gravity acts on the whole of it. coefficients that could not have
// come from a real bicycle are refused rather than quietly estimated at.
//
// See docs/specs/measurement.md §Estimated power.
func EstimateSeries(samples []Sample, totalMassKG float64, coefficients Coefficients) ([]Estimate, bool) {
	if !coefficients.Valid() {
		return nil, false
	}

	return estimateSeries(samples, totalMassKG, coefficients)
}

// estimateSeries is EstimateSeries' body, split out so the coefficient
// validation above sits beside the one caller that needs it.
func estimateSeries(samples []Sample, totalMassKG float64, coefficients Coefficients) ([]Estimate, bool) {
	if totalMassKG <= 0 || len(samples) < 2 {
		return nil, false
	}
	estimates := make([]Estimate, len(samples))
	bounds := distanceStretches(samples, DefaultMaxGap)
	windowMetres := gradeWindowMetres(samples)
	known := false
	// Each sample's own window speed, kept so the inertial term can
	// differentiate this one against a sample a baseline behind it.
	speeds := make([]float64, len(samples))
	speedKnown := make([]bool, len(samples))
	for index := 1; index < len(samples); index++ {
		// The step to the sample before is what marks a pause; the window the
		// speed and grade are then measured over never crosses one.
		step := samples[index].At.Sub(samples[index-1].At)
		if step <= 0 || step > DefaultMaxGap {
			continue
		}
		low, high := centredWindow(samples, index, bounds, windowMetres)
		span := samples[high].At.Sub(samples[low].At).Seconds()
		run := samples[high].DistanceMetres - samples[low].DistanceMetres
		if span <= 0 || run < 0 {
			continue
		}
		speed := run / span
		speeds[index], speedKnown[index] = speed, true
		acceleration := accelerationAt(samples, speeds, speedKnown, index, bounds)
		// Physics, not the numerical clamp below: no pedalling reads no power,
		// checked before the clamp has any say.
		if samples[index].HasCadence && samples[index].CadenceRPM == 0 {
			estimates[index] = Estimate{Watts: 0, Known: true}
		} else {
			temperature := float64(defaultTemperatureCelsius)
			if samples[high].HasTemperature {
				temperature = samples[high].TemperatureCelsius
			}
			density := airDensity(samples[high].AltitudeMetres, temperature)
			estimates[index] = Estimate{
				Watts: watts(speed, slope(samples[low], samples[high]), totalMassKG, density, acceleration, coefficients),
				Known: true,
			}
		}
		known = true
	}

	return estimates, known
}

// accelerationAt differentiates the window speed at index against the first
// sample at least accelerationBaseline behind it, and reports nought where the
// recording does not reach back that far. A sample with no window speed of
// its own -- a duplicate timestamp, a backward-distance glitch, the stretch's
// own opening sample -- is skipped rather than treated as the baseline; the
// walk stops only at the stretch's own start, the actual recording gap
// bounds[index] already names for this sample.
func accelerationAt(samples []Sample, speeds []float64, speedKnown []bool, index int, bounds []Stretch) float64 {
	for back := index - 1; back >= bounds[index].First; back-- {
		if !speedKnown[back] {
			continue
		}
		if elapsed := samples[index].At.Sub(samples[back].At); elapsed >= accelerationBaseline {
			return (speeds[index] - speeds[back]) / elapsed.Seconds()
		}
	}

	return 0
}

// watts is the model itself: what it costs to climb the grade, roll along it,
// push the air aside and get back up to speed, all at once, divided by the
// drivetrain that carries it to the road. Clamped at zero: a rider
// freewheeling down a hill is putting nothing in, and the model has no way to
// say they are taking something out.
func watts(speed, grade, totalMassKG, density, accelerationMSS float64, coefficients Coefficients) float64 {
	weight := totalMassKG * gravity
	force := weight*grade + weight*coefficients.RollingResistance +
		0.5*density*coefficients.DragArea*math.Abs(speed)*speed +
		(totalMassKG+rotationalMassKG)*accelerationMSS

	return max(force*speed/drivetrainEfficiency, 0)
}

// distanceStretches is Stretches further split wherever a sample's own
// distance runs backward: an odometer reset marks a boundary a window must
// not cross the same way a recording gap already does, or centredWindow
// combines distance and altitude from before and after the reset into one
// bogus speed and grade. A duplicate or backward clock is the same kind of
// boundary, and for the same reason: gradeWindowMetres and the per-sample
// step check both already require a strictly positive step, not merely one
// no wider than a gap.
func distanceStretches(samples []Sample, maxGap time.Duration) []Stretch {
	stretches := make([]Stretch, len(samples))
	for first := 0; first < len(samples); {
		past := first + 1
		for past < len(samples) {
			step := samples[past].At.Sub(samples[past-1].At)
			if step <= 0 || step > maxGap || samples[past].DistanceMetres < samples[past-1].DistanceMetres {
				break
			}
			past++
		}
		for index := first; index < past; index++ {
			stretches[index] = Stretch{First: first, Past: past}
		}
		first = past
	}

	return stretches
}

// centredWindow is the span of samples around one sample that covers
// windowMetres of distance, or as much of it as that stretch of recording
// holds. Both bounds are inclusive.
func centredWindow(samples []Sample, index int, bounds []Stretch, windowMetres float64) (low, high int) {
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

// PedallingMean is the mean estimated power over the samples the rider was
// pedalling through, and the share of the ride's known samples that was: a
// bicycle produces nothing while it coasts, so the ride's own figure is the
// power while pedalling, and the share says how much of the ride that was.
// ok is false where nothing was estimated at all.
func PedallingMean(samples []Sample, estimates []Estimate) (watts, share float64, ok bool) {
	if len(samples) != len(estimates) {
		return 0, 0, false
	}
	var known, pedalling int
	var total float64
	for index, estimate := range estimates {
		if !estimate.Known {
			continue
		}
		known++
		if samples[index].HasCadence && samples[index].CadenceRPM == 0 {
			continue
		}
		pedalling++
		total += estimate.Watts
	}
	if known == 0 {
		return 0, 0, false
	}
	if pedalling == 0 {
		return 0, 0, true
	}

	return total / float64(pedalling), float64(pedalling) / float64(known), true
}
