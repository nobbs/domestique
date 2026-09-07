package trainingload

import (
	"math"
	"time"
)

// Banister's exponential weighting. The rider's sex is not among the profile's
// parameters, so the coefficients for men are used for everyone: they scale
// every ride by the same constant, and TRIMP is read against a rider's own
// other rides rather than against anybody else's.
const (
	trimpFactor   = 0.64
	trimpExponent = 1.92
)

// TRIMP is Banister's training impulse: how long the ride lasted, weighted by
// how much of the rider's heart-rate reserve it held, in minutes. It needs a
// maximum and a resting rate to have a reserve to measure against at all.
func TRIMP(samples []Sample, maxHeartRate, restingHeartRate float64) (float64, bool) {
	// Both rates, not just a positive difference: a resting rate of zero is a
	// parameter the rider has not entered, and the reserve it implies is not one.
	reserve := maxHeartRate - restingHeartRate
	if restingHeartRate <= 0 || reserve <= 0 {
		return 0, false
	}
	impulse := 0.0
	held := 0.0
	forEachHeld(samples, func(heartRate, seconds float64) {
		held += seconds
		// Clamped: a rate above the entered maximum is a maximum entered too
		// low, and an unclamped exponential turns that into a wild number.
		fraction := math.Min(math.Max((heartRate-restingHeartRate)/reserve, 0), 1)
		impulse += (seconds / 60) * fraction * trimpFactor * math.Exp(trimpExponent*fraction)
	})
	if held <= 0 {
		return 0, false
	}

	return impulse, true
}

// HeartRateTSS scores the ride the way power TSS does but from heart rate: an
// hour held at the lactate threshold is 100. It needs a threshold rate, which is
// the only rate that makes the score mean the same thing between riders, and a
// resting rate to measure the reserve against.
//
// The intensity is the share of the rider's threshold reserve their mean rate
// held, not their mean rate over their threshold. Heart rate does not fall to
// zero as power does — an idle rider still beats at their resting rate — so the
// bare ratio scores an easy ride as though it were most of a threshold effort.
func HeartRateTSS(samples []Sample, thresholdHeartRate, restingHeartRate float64) (float64, bool) {
	reserve := thresholdHeartRate - restingHeartRate
	if restingHeartRate <= 0 || reserve <= 0 {
		return 0, false
	}
	mean, seconds := meanHeld(samples)
	if seconds <= 0 {
		return 0, false
	}
	// Clamped: a mean below the entered resting rate is a resting rate entered
	// too high, and a negative reserve is not an intensity.
	intensity := math.Max((mean-restingHeartRate)/reserve, 0)

	return seconds / 3600 * intensity * intensity * 100, true
}

// normalizedPowerWindow is the rolling average normalized power is built on,
// which is what makes it punish surges rather than average them away.
const normalizedPowerWindow = 30 * time.Second

// Power is what a ride's power samples and a rider's threshold power say about
// it. Its zero value is a ride that yielded none.
type Power struct {
	NormalizedWatts float64
	IntensityFactor float64
	TSS             float64
}

// PowerLoad works out normalized power, the intensity factor and the training
// stress score. It needs a functional threshold power: without one there is
// nothing to call an intensity, and the score would mean nothing.
//
// The samples must be real, measured power. An estimate from the track is a
// different kind of number and never feeds this.
func PowerLoad(samples []Sample, functionalThresholdPower float64) (Power, bool) {
	if functionalThresholdPower <= 0 {
		return Power{}, false
	}
	rolling, seconds, ok := rollingFourthPowerMean(samples)
	if !ok || seconds <= 0 {
		return Power{}, false
	}
	normalized := round4(rolling)
	intensity := normalized / functionalThresholdPower

	return Power{
		NormalizedWatts: normalized,
		IntensityFactor: intensity,
		TSS:             seconds * normalized * intensity / (functionalThresholdPower * 3600) * 100,
	}, true
}

// rollingFourthPowerMean averages the fourth power of a rolling thirty-second
// mean, which is normalized power one fourth root from the end. It reports how
// long the ride's samples stood, which the stress score is scaled by, and
// whether the ride ever held a full window — a ride shorter than one has no
// rolling mean to raise, and its own average is not normalized power.
func rollingFourthPowerMean(samples []Sample) (mean, seconds float64, ok bool) {
	// A sample with less than a full window behind it has no rolling mean to
	// raise, and a silence breaks the series rather than being averaged across.
	total := 0.0
	weight := 0.0
	start := 0
	// integral[k] is the power integrated from the first sample to samples[k].
	integral := make([]float64, len(samples))
	for index := 1; index < len(samples); index++ {
		held := samples[index].At.Sub(samples[index-1].At)
		if held <= 0 || held > maxSampleGap {
			// A silence breaks the series: what follows starts a new window.
			integral[index] = integral[index-1]
			start = index

			continue
		}
		seconds += held.Seconds()
		integral[index] = integral[index-1] + samples[index-1].Value*held.Seconds()
		for start+1 < index &&
			samples[index].At.Sub(samples[start+1].At) >= normalizedPowerWindow {
			start++
		}
		span := samples[index].At.Sub(samples[start].At)
		if span < normalizedPowerWindow {
			continue
		}
		rolling := (integral[index] - integral[start]) / span.Seconds()
		total += math.Pow(rolling, 4) * held.Seconds()
		weight += held.Seconds()
	}
	if weight <= 0 {
		return 0, seconds, false
	}

	return total / weight, seconds, true
}
