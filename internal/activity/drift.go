package activity

import (
	"math"
	"time"

	"github.com/nobbs/domestique/internal/trainingload"
)

// DecouplingMinimumDuration is what a decoupling figure needs of a ride before
// it means anything: Friel's ratio compares two halves of one aerobic effort,
// and a shorter ride splits into halves too brief to have drifted.
//
// Measured across the recorded samples, first to last, so a ride recorded once
// a second must run a second past the hour to clear it. The boundary is a
// judgement either way and nothing turns on which side of it one second falls.
const DecouplingMinimumDuration = time.Hour

// HeatDriftBandLowerShare and HeatDriftBandUpperShare bound the endurance band
// a reading is taken in, as shares of threshold power. Heart rate answers
// temperature at a steady aerobic effort; above the band the effort drives it.
const (
	HeatDriftBandLowerShare = 0.55
	HeatDriftBandUpperShare = 0.75
	// heatDriftMinimumSamples keeps a reading off a handful of samples that
	// happened to pass through the band.
	heatDriftMinimumSamples = 300
)

// Decoupling is how much the ride's power-to-heart-rate ratio fell away over
// its second half, as a percentage of its first. Positive is the usual
// direction: the same watts cost more beats later on.
//
// Measured power only, and only over a ride long enough to have two halves
// worth comparing. It describes a steady aerobic ride; over intervals the two
// halves are different efforts and the number says nothing about drift.
type Decoupling struct {
	Percent float64
	Known   bool
}

// HeatDrift is what one ride says about riding warm: the heart rate it held in
// the rider's endurance band, and the temperature it was recorded at. One ride
// is a point rather than a trend — the trend is these points over a season.
type HeatDrift struct {
	HeartRateBPM       float64
	TemperatureCelsius float64
	Samples            int
	Known              bool
}

// Decoupling works the ride's aerobic decoupling out from its measured power
// and the cleaned heart rate the caller passes in. The cleaned series and not
// this ride's own: a spike above the rider's maximum lands in one half and
// would be read as drift.
func (s *RideSamples) Decoupling(heartRate []trainingload.Sample) Decoupling {
	if len(s.Power) == 0 || len(heartRate) == 0 {
		return Decoupling{}
	}
	start, end := s.Power[0].At, s.Power[len(s.Power)-1].At
	if end.Sub(start) < DecouplingMinimumDuration {
		return Decoupling{}
	}
	middle := start.Add(end.Sub(start) / 2)
	first, firstOK := ratioOver(s.Power, heartRate, start, middle)
	second, secondOK := ratioOver(s.Power, heartRate, middle, end.Add(time.Nanosecond))
	if !firstOK || !secondOK || first == 0 {
		return Decoupling{}
	}
	percent := (first - second) / first * 100

	return Decoupling{Percent: percent, Known: !math.IsNaN(percent) && !math.IsInf(percent, 0)}
}

// ratioOver is mean power over mean heart rate across one half of the ride.
// Both series are the ride's own, so a half that caught no heart rate has no
// ratio rather than an infinite one.
func ratioOver(power, heartRate []trainingload.Sample, from, to time.Time) (float64, bool) {
	meanPower, powerOK := meanBetween(power, from, to)
	meanHeartRate, heartRateOK := meanBetween(heartRate, from, to)
	if !powerOK || !heartRateOK || meanHeartRate == 0 {
		return 0, false
	}

	return meanPower / meanHeartRate, true
}

// meanBetween averages the samples recorded in [from, to).
func meanBetween(samples []trainingload.Sample, from, to time.Time) (float64, bool) {
	total, counted := 0.0, 0
	for index := range samples {
		at := samples[index].At
		if at.Before(from) || !at.Before(to) {
			continue
		}
		total += samples[index].Value
		counted++
	}
	if counted == 0 {
		return 0, false
	}

	return total / float64(counted), true
}

// HeatDrift reads the ride's endurance-band heart rate and the temperature it
// was held at, over the cleaned heart rate the caller passes in. Measured power
// places the band: an estimate carries a per-ride bias, which would put the
// same effort in different bands on different rides and make a season of these
// points incomparable.
func (s *RideSamples) HeatDrift(
	cleanedHeartRate []trainingload.Sample, thresholdPowerWatts float64,
) HeatDrift {
	if thresholdPowerWatts <= 0 || len(s.Power) == 0 ||
		len(cleanedHeartRate) == 0 || len(s.Temperature) == 0 {
		return HeatDrift{}
	}
	lower := thresholdPowerWatts * HeatDriftBandLowerShare
	upper := thresholdPowerWatts * HeatDriftBandUpperShare
	heartRate := byTime(cleanedHeartRate)
	temperature := byTime(s.Temperature)
	totalHeartRate, totalTemperature, counted := 0.0, 0.0, 0
	for index := range s.Power {
		sample := s.Power[index]
		if sample.Value < lower || sample.Value > upper {
			continue
		}
		beats, hasBeats := heartRate[sample.At.Unix()]
		degrees, hasDegrees := temperature[sample.At.Unix()]
		if !hasBeats || !hasDegrees {
			continue
		}
		totalHeartRate += beats
		totalTemperature += degrees
		counted++
	}
	if counted < heatDriftMinimumSamples {
		return HeatDrift{}
	}

	return HeatDrift{
		HeartRateBPM:       totalHeartRate / float64(counted),
		TemperatureCelsius: totalTemperature / float64(counted),
		Samples:            counted,
		Known:              true,
	}
}

// byTime indexes a series by the second it was recorded at, which is the
// resolution the records are stored at and so the only way two sensors' samples
// are known to describe the same moment.
func byTime(samples []trainingload.Sample) map[int64]float64 {
	indexed := make(map[int64]float64, len(samples))
	for index := range samples {
		indexed[samples[index].At.Unix()] = samples[index].Value
	}

	return indexed
}
