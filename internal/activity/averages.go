package activity

import "github.com/nobbs/domestique/internal/trainingload"

// RideAverages is what a ride's own sensors came to, with no rider profile
// involved: the plain figures a rider reads before any training load. Each is
// present only where the ride carried that sensor, so a bicycle with no meter
// has no average power rather than an average of nothing.
//
// Every one is a mean over the recorded samples, in the order they were
// recorded. A reading of zero is a reading — a stopped rider's cadence — and
// counts towards the mean like any other.
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
	averages.HeartRateBPM, averages.MaxHeartRateBPM, averages.HasHeartRate = meanAndPeak(s.HeartRate)
	averages.CadenceRPM, _, averages.HasCadence = meanAndPeak(s.Cadence)
	averages.PowerWatts, _, averages.HasPower = meanAndPeak(s.Power)

	return averages
}

// meanAndPeak is one series' mean and highest reading. known is false for a
// series no sample carried, which is what tells an unfitted sensor from one
// that read nought.
func meanAndPeak(samples []trainingload.Sample) (mean, peak float64, known bool) {
	if len(samples) == 0 {
		return 0, 0, false
	}
	total := 0.0
	peak = samples[0].Value
	for index := range samples {
		total += samples[index].Value
		peak = max(peak, samples[index].Value)
	}

	return total / float64(len(samples)), peak, true
}

// RideMetrics is everything one derivation writes about a ride: the load
// figures the rider's profile shapes, and the sensor averages it does not.
type RideMetrics struct {
	Load     trainingload.Metrics
	Averages RideAverages
}

// Derived reports whether anything at all came out, which is what decides
// between storing a row and storing none.
func (m *RideMetrics) Derived() bool {
	return m.Load.Derived() || m.Averages.Any()
}
