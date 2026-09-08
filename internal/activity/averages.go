package activity

import (
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/trainingload"
)

// RideAverages is what a ride's own sensors came to, with no rider profile
// involved: the plain figures a rider reads before any training load. Each is
// present only where the ride carried that sensor, so a bicycle with no meter
// has no average power rather than an average of nothing.
//
// Every one is a mean over the recorded samples, in the order they were
// recorded. A cadence of zero is the rider not pedalling rather than pedalling
// slowly, and is left out of the mean, which is what every other platform
// reports and what makes this figure comparable to theirs. A measured power of
// zero is left in: freewheeling is part of what a ride averaged.
type RideAverages struct {
	HeartRateBPM    float64
	MaxHeartRateBPM float64
	CadenceRPM      float64
	PowerWatts      float64
	HasHeartRate    bool
	HasCadence      bool
	HasPower        bool
}

// Any reports whether the ride yielded a single one of these figures.
func (a *RideAverages) Any() bool {
	return a.HasHeartRate || a.HasCadence || a.HasPower
}

// Averages works the ride's plain sensor figures out from its stored samples.
func (s *RideSamples) Averages() RideAverages {
	averages := RideAverages{}
	averages.HeartRateBPM, averages.MaxHeartRateBPM, averages.HasHeartRate = meanAndPeak(s.HeartRate, countZero)
	averages.CadenceRPM, _, averages.HasCadence = meanAndPeak(s.Cadence, skipZero)
	averages.PowerWatts, _, averages.HasPower = meanAndPeak(s.Power, countZero)

	return averages
}

// How a zero reading is treated, named at the call site rather than passed as a
// bare true: which of the two a sensor wants is the whole of what differs.
const (
	skipZero  = true
	countZero = false
)

// meanAndPeak is one series' mean and highest reading, over the samples that
// count towards it. known is false for a series that yielded none, which is
// what tells an unfitted sensor from one that read nought throughout.
//
// The peak is over every sample either way: a zero is never the highest reading
// of a series that holds anything else.
func meanAndPeak(samples []trainingload.Sample, skipZeroReadings bool) (mean, peak float64, known bool) {
	if len(samples) == 0 {
		return 0, 0, false
	}
	total, counted := 0.0, 0
	peak = samples[0].Value
	for index := range samples {
		value := samples[index].Value
		peak = max(peak, value)
		if skipZeroReadings && value == 0 {
			continue
		}
		total += value
		counted++
	}
	if counted == 0 {
		return 0, 0, false
	}

	return total / float64(counted), peak, true
}

// RideMetrics is everything one derivation writes about a ride: the load
// figures the rider's profile shapes, the sensor averages it does not, and the
// estimate's own quality diagnostics. A row derived before those existed holds
// an estimate and no quality, so the quality carries its own presence.
type RideMetrics struct {
	Load               trainingload.Metrics
	Averages           RideAverages
	EstimateQuality    measure.Quality
	Decoupling         Decoupling
	HeatDrift          HeatDrift
	HasEstimateQuality bool
}

// Derived reports whether anything at all came out, which is what decides
// between storing a row and storing none.
func (m *RideMetrics) Derived() bool {
	return m.Load.Derived() || m.Averages.Any() || m.Decoupling.Known || m.HeatDrift.Known
}
