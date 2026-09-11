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
	// TotalMassKG is the rider and their bicycle together. Nothing in this
	// package reads it: it is here because a derivation is stale when any
	// profile value it was worked out against has changed, and the estimated
	// power a derivation also produces is worked out from this one.
	TotalMassKG float64
}

// InputsOf reads the profile values a derivation is worked out against. A
// parameter the rider has not entered is zero here, which every rule below
// reads as "not enough to work this out" rather than as a value.
func InputsOf(profile *rider.Profile) Inputs {
	return Inputs{
		MaxHeartRateBPM:               profile.MaxHeartRateBPM.Number,
		RestingHeartRateBPM:           profile.RestingHeartRateBPM.Number,
		ThresholdHeartRateBPM:         profile.ThresholdHeartRateBPM.Number,
		FunctionalThresholdPowerWatts: profile.FunctionalThresholdPowerWatts.Number,
		// Both halves or neither: a total missing one of them is not a total,
		// and half a mass would estimate half a rider's power.
		TotalMassKG: totalMass(profile),
	}
}

func totalMass(profile *rider.Profile) float64 {
	if !profile.RiderMassKG.Set || !profile.BikeMassKG.Set {
		return 0
	}

	return profile.RiderMassKG.Number + profile.BikeMassKG.Number
}

// Metrics is everything one ride's samples and one profile yield. Each part is
// present only where both the sensor and the parameters it needs were there.
type Metrics struct {
	Inputs Inputs
	Zones  Zones
	// TRIMP and HeartRateTSS are the two load scales, kept side by side rather
	// than reconciled: they answer different questions and neither converts to
	// the other.
	TRIMP        float64
	HeartRateTSS float64
	Power        Power
	// EstimatedPowerWatts is the ride's average estimated power, for a bicycle
	// with no meter. It is never an input to anything above: an estimate must
	// not become a measurement by being averaged into one.
	EstimatedPowerWatts float64
	HasZones            bool
	HasTRIMP            bool
	HasHeartRateTSS     bool
	HasPower            bool
	HasEstimatedPower   bool
}

// Derived reports whether anything at all came out, which is what decides
// between storing a row and storing none.
func (m *Metrics) Derived() bool {
	return m.HasZones || m.HasTRIMP || m.HasHeartRateTSS || m.HasPower || m.HasEstimatedPower
}

// Derive works out everything one ride yields. heartRate and power are that
// ride's samples of each sensor, in recorded order, with the samples that
// carried no reading left out rather than passed as zero. movingSeconds is
// the ride's own moving time, which a figure's series coverage is judged
// against; a figure whose series measurably held for less than
// MinSeriesCoverage of it is withheld rather than served understated. Zero
// or negative leaves coverage unmeasured rather than failed, so a caller that
// cannot supply it yet gets today's behaviour rather than everything withheld.
func Derive(heartRate, power []Sample, movingSeconds float64, inputs Inputs) Metrics {
	metrics := Metrics{Inputs: inputs}
	// A strap that wrote nought took no reading there; one that wrote nought
	// throughout covered nothing, and is judged so rather than left unjudged.
	strapWroteNought := len(heartRate) > 0
	heartRate = HeartRateReadings(heartRate)
	strapWroteNought = strapWroteNought && len(heartRate) == 0
	if bounds, ok := BoundsFrom(inputs.ThresholdHeartRateBPM, inputs.MaxHeartRateBPM); ok && len(heartRate) > 1 {
		zones := TimeInZones(heartRate, bounds)
		metrics.Zones, metrics.HasZones = zones, zones.Total() > 0
	}
	metrics.TRIMP, metrics.HasTRIMP = TRIMP(heartRate, inputs.MaxHeartRateBPM, inputs.RestingHeartRateBPM)
	metrics.HeartRateTSS, metrics.HasHeartRateTSS = HeartRateTSS(heartRate, inputs.ThresholdHeartRateBPM, inputs.RestingHeartRateBPM)
	metrics.Power, metrics.HasPower = PowerLoad(power, inputs.FunctionalThresholdPowerWatts)

	if coverage, ok := SeriesCoverage(heartRate, movingSeconds); (ok && coverage < MinSeriesCoverage) ||
		(strapWroteNought && movingSeconds > 0) {
		metrics.HasZones, metrics.HasTRIMP, metrics.HasHeartRateTSS = false, false, false
	}
	if coverage, ok := SeriesCoverage(power, movingSeconds); ok && coverage < MinSeriesCoverage {
		metrics.HasPower = false
	}

	return metrics
}
