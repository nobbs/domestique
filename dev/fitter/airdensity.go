package main

import "math"

// standardAirDensity is sea-level air density at 15°C, ISA standard
// conditions — the fallback for a sample whose FIT temperature channel is
// absent. Most head units carry it; where one does not, this constant is
// what the window's drag term uses instead of a per-ride figure.
const standardAirDensity = 1.225

const (
	seaLevelPressurePa        = 101325.0
	seaLevelTemperatureK      = 288.15
	temperatureLapseKPerM     = 0.0065
	specificGasConstantDryAir = 287.05
	barometricExponent        = 5.255
)

// airDensityFor derives ρ from a sample's recorded temperature and altitude
// via the International Standard Atmosphere barometric formula, rather than
// assuming a constant: a bias here would otherwise be absorbed into CdA,
// exactly the confound the issue warns a missing surface term causes for
// power.
func airDensityFor(s *sampleRow) float64 {
	if !s.HasTemperature || !s.HasAltitude {
		return standardAirDensity
	}

	temperatureK := s.TemperatureC + 273.15
	pressure := seaLevelPressurePa * math.Pow(1-temperatureLapseKPerM*s.AltitudeM/seaLevelTemperatureK, barometricExponent)

	return pressure / (specificGasConstantDryAir * temperatureK)
}

// meanAirDensity is a group's own representative ρ for the coefficient
// file's air_density_kg_per_m3 field: the forward model predicts future
// rides under conditions it cannot know, so the mean of what this group was
// actually fitted against is a more representative constant than a blanket
// standard-atmosphere figure.
func meanAirDensity(windows []coastingWindow) float64 {
	if len(windows) == 0 {
		return standardAirDensity
	}

	var sum float64
	for _, w := range windows {
		sum += w.AirDensity
	}

	return sum / float64(len(windows))
}
