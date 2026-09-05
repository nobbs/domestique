package main

import (
	"fmt"
	"os"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/pelletier/go-toml/v2"
)

// rawCoefficients is the TOML shape this tool reads and prints. The service
// keeps its pair in the state database; only the offline fitter still passes
// one around as a file.
type rawCoefficients struct {
	CalibrationCutoff string  `toml:"calibration_cutoff"`
	SecondsPerKM      float64 `toml:"seconds_per_km"`
	SecondsPerAscentM float64 `toml:"seconds_per_ascent_m"`
	EvaluatedRides    int     `toml:"evaluated_rides"`
	BiasPercent       float64 `toml:"bias_percent"`
	MAEPercent        float64 `toml:"mae_percent"`
	P90Percent        float64 `toml:"p90_percent"`

	TrainingWindowMonths int `toml:"training_window_months"`
}

// loadCoefficients reads a profile file. A file still carrying the retired
// physics keys (mass_kg, power_watts, cda_m2, crr) loads: they are ignored.
func loadCoefficients(path string) (ridemodel.Coefficients, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is this tool's own -coefficients flag.
	if err != nil {
		return ridemodel.Coefficients{}, fmt.Errorf("reading coefficient file: %w", err)
	}
	var raw rawCoefficients
	if unmarshalErr := toml.Unmarshal(data, &raw); unmarshalErr != nil {
		return ridemodel.Coefficients{}, fmt.Errorf("parsing coefficient file: %w", unmarshalErr)
	}
	coefficients := ridemodel.Coefficients{
		CalibrationCutoff:    raw.CalibrationCutoff,
		SecondsPerKM:         raw.SecondsPerKM,
		SecondsPerAscentM:    raw.SecondsPerAscentM,
		EvaluatedRides:       raw.EvaluatedRides,
		BiasPercent:          raw.BiasPercent,
		MAEPercent:           raw.MAEPercent,
		P90Percent:           raw.P90Percent,
		TrainingWindowMonths: raw.TrainingWindowMonths,
	}.WithFingerprint()
	if validateErr := coefficients.Validate(); validateErr != nil {
		return ridemodel.Coefficients{}, fmt.Errorf("coefficient file: %w", validateErr)
	}

	return coefficients, nil
}
