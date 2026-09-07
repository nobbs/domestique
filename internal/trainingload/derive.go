package trainingload

import "github.com/nobbs/domestique/internal/rider"

// Inputs are the profile values a derivation reads. They are recorded on the
// row beside the numbers, so a row worked out against a profile the rider has
// since changed is recognisable as stale without guessing.
type Inputs struct {
	MaxHeartRateBPM               float64
	RestingHeartRateBPM           float64
	ThresholdHeartRateBPM         float64
	FunctionalThresholdPowerWatts float64
}

// InputsOf reads the four parameters a derivation uses out of a profile. A
// parameter the rider has not entered is zero here, which every rule below
// reads as "not enough to work this out" rather than as a value.
func InputsOf(profile *rider.Profile) Inputs {
	return Inputs{
		MaxHeartRateBPM:               profile.MaxHeartRateBPM.Number,
		RestingHeartRateBPM:           profile.RestingHeartRateBPM.Number,
		ThresholdHeartRateBPM:         profile.ThresholdHeartRateBPM.Number,
		FunctionalThresholdPowerWatts: profile.FunctionalThresholdPowerWatts.Number,
	}
}

// Metrics is everything one ride's samples and one profile yield. Each part is
// present only where both the sensor and the parameters it needs were there.
type Metrics struct {
	Inputs Inputs
	Zones  Zones
	// TRIMP and HeartRateTSS are the two load scales, kept side by side rather
	// than reconciled: they answer different questions and neither converts to
	// the other.
	TRIMP           float64
	HeartRateTSS    float64
	Power           Power
	HasZones        bool
	HasTRIMP        bool
	HasHeartRateTSS bool
	HasPower        bool
}

// Derived reports whether anything at all came out, which is what decides
// between storing a row and storing none.
func (m *Metrics) Derived() bool {
	return m.HasZones || m.HasTRIMP || m.HasHeartRateTSS || m.HasPower
}

// Derive works out everything one ride yields. heartRate and power are that
// ride's samples of each sensor, in recorded order, with the samples that
// carried no reading left out rather than passed as zero.
func Derive(heartRate, power []Sample, inputs Inputs) Metrics {
	metrics := Metrics{Inputs: inputs}
	if bounds, ok := BoundsFrom(inputs.ThresholdHeartRateBPM, inputs.MaxHeartRateBPM); ok && len(heartRate) > 1 {
		zones := TimeInZones(heartRate, bounds)
		metrics.Zones, metrics.HasZones = zones, zones.Total() > 0
	}
	metrics.TRIMP, metrics.HasTRIMP = TRIMP(heartRate, inputs.MaxHeartRateBPM, inputs.RestingHeartRateBPM)
	metrics.HeartRateTSS, metrics.HasHeartRateTSS = HeartRateTSS(heartRate, inputs.ThresholdHeartRateBPM)
	metrics.Power, metrics.HasPower = PowerLoad(power, inputs.FunctionalThresholdPowerWatts)

	return metrics
}
