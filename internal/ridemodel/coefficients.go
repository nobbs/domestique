// Package ridemodel is the forward model that turns a stage's geometry and a
// calibrated coefficient pair into a predicted moving time. It is a pure
// function of its inputs, and dev/fitter's benchmark runs exactly this model.
package ridemodel

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// Coefficients are the values the model can legitimately vary: the two fitted
// terms, and the metadata describing the fit that produced them. They arrive as
// one file, loaded once at startup.
type Coefficients struct {
	Fingerprint       string
	CalibrationCutoff string // "2025-08-01"; the last calibration ride's date, recorded as-is rather than computed from anything at load time
	SecondsPerKM      float64
	SecondsPerAscentM float64
	// EvaluatedRides, BiasPercent, MAEPercent, and P90Percent are the profile's
	// measured unseen-route error, pooled across rolling-origin folds. Optional:
	// EvaluatedRides == 0 is the sentinel for "not measured", which a real
	// evaluation can never produce.
	EvaluatedRides int
	BiasPercent    float64
	MAEPercent     float64
	P90Percent     float64
	// TrainingWindowMonths is how far back the fit that produced SecondsPerKM and
	// SecondsPerAscentM reached. Nothing here reads it, but without it a profile
	// cannot be reproduced from its own metadata. Zero means all history.
	TrainingWindowMonths int
}

// HasValidation reports whether the loaded file carries a measured
// unseen-route benchmark result.
//
//nolint:gocritic // value receiver: Coefficients is immutable once loaded, and a pointer would let a caller mutate the shared instance mid-prediction.
func (c Coefficients) HasValidation() bool {
	return c.EvaluatedRides > 0
}

// rawCoefficients is the TOML shape of the profile #239 emits.
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

// Load reads, parses, and validates a coefficient file, returning an error for
// a missing, malformed or implausible one; the caller decides whether that stops
// the service or only prediction. Retired physics keys in the file are ignored.
func Load(path string) (Coefficients, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is an operator-configured absolute file, not user input.
	if err != nil {
		return Coefficients{}, fmt.Errorf("ridemodel: reading coefficient file: %w", err)
	}

	var raw rawCoefficients
	if unmarshalErr := toml.Unmarshal(data, &raw); unmarshalErr != nil {
		return Coefficients{}, fmt.Errorf("ridemodel: parsing coefficient file: %w", unmarshalErr)
	}

	coefficients, err := raw.build()
	if err != nil {
		return Coefficients{}, err
	}

	// modelVersion is mixed into the hash, not just the file's bytes: a code
	// upgrade that changes a versioned constant must invalidate a cached
	// prediction even when the file is byte-for-byte unchanged.
	// Hashed from the two terms rather than the file's bytes, so a retired key
	// or a reformatted file leaves every cached prediction standing.
	coefficients.Fingerprint = fingerprintOf(modelVersion, []byte(termsOf(coefficients.SecondsPerKM, coefficients.SecondsPerAscentM)))

	return coefficients, nil
}

// termsOf is the canonical spelling of what the equation reads from a profile.
func termsOf(secondsPerKM, secondsPerAscentM float64) string {
	return strconv.FormatFloat(secondsPerKM, 'g', -1, 64) + "\n" + strconv.FormatFloat(secondsPerAscentM, 'g', -1, 64)
}

// fingerprintOf hashes version and data with version's length written ahead of
// it, so the two can never be reinterpreted as a different (version, data) pair
// that concatenates to the same bytes.
func fingerprintOf(version string, data []byte) string {
	hash := sha256.New()
	var versionLength [8]byte
	binary.BigEndian.PutUint64(versionLength[:], uint64(len(version)))
	// hash.Hash.Write never returns an error — the interface carries one only
	// because it embeds io.Writer.
	_, _ = hash.Write(versionLength[:])
	_, _ = hash.Write([]byte(version))
	_, _ = hash.Write(data)

	return hex.EncodeToString(hash.Sum(nil))
}

func (r *rawCoefficients) build() (Coefficients, error) {
	if math.IsNaN(r.SecondsPerKM) || math.IsInf(r.SecondsPerKM, 0) ||
		math.IsNaN(r.SecondsPerAscentM) || math.IsInf(r.SecondsPerAscentM, 0) {
		return Coefficients{}, errors.New("ridemodel: the coefficient terms must be finite")
	}
	if r.SecondsPerKM <= 0 {
		return Coefficients{}, errors.New("ridemodel: seconds_per_km must be positive")
	}
	if r.SecondsPerAscentM <= 0 {
		return Coefficients{}, errors.New("ridemodel: seconds_per_ascent_m must be positive")
	}
	if _, parseErr := time.Parse(time.DateOnly, r.CalibrationCutoff); parseErr != nil {
		return Coefficients{}, fmt.Errorf("ridemodel: calibration_cutoff must be a date in %s form: %w", time.DateOnly, parseErr)
	}
	// Optional, but a value present must be one that could mean something:
	// EvaluatedRides is a count and the two percentages are magnitudes, so none
	// can be negative. BiasPercent is signed and gets no such check.
	if r.EvaluatedRides < 0 {
		return Coefficients{}, errors.New("ridemodel: evaluated_rides must not be negative")
	}
	if r.MAEPercent < 0 {
		return Coefficients{}, errors.New("ridemodel: mae_percent must not be negative")
	}
	if r.P90Percent < 0 {
		return Coefficients{}, errors.New("ridemodel: p90_percent must not be negative")
	}
	if r.TrainingWindowMonths < 0 {
		return Coefficients{}, errors.New("ridemodel: training_window_months must not be negative")
	}
	// A partially-updated file — percentages set without evaluated_rides — must
	// not silently load and drop the metadata: HasValidation() would read it as
	// "not measured" and serve none of it.
	if r.EvaluatedRides == 0 && (r.BiasPercent != 0 || r.MAEPercent != 0 || r.P90Percent != 0) {
		return Coefficients{}, errors.New(
			"ridemodel: bias_percent, mae_percent, and p90_percent require evaluated_rides",
		)
	}

	return Coefficients{
		CalibrationCutoff: r.CalibrationCutoff,
		SecondsPerKM:      r.SecondsPerKM,
		SecondsPerAscentM: r.SecondsPerAscentM,
		EvaluatedRides:    r.EvaluatedRides,
		BiasPercent:       r.BiasPercent,
		MAEPercent:        r.MAEPercent,
		P90Percent:        r.P90Percent,

		TrainingWindowMonths: r.TrainingWindowMonths,
	}, nil
}
