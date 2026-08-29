// Package ridemodel is the forward physical model that turns a stage's geometry
// and a set of hybrid coefficients into a predicted moving time. It is a pure
// function of its inputs, and dev/fitter's benchmark runs exactly this model.
package ridemodel

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/nobbs/domestique/internal/surface"
)

// Physical plausibility bounds for a loaded coefficient file: wide enough to
// admit any rider and machine, narrow enough to catch a transposed unit or a
// fit that did not converge.
const (
	minMassKG     = 20.0
	maxMassKG     = 300.0
	minPowerWatts = 20.0
	maxPowerWatts = 1000.0
	// minCdAM2 is below anything a person on a bicycle presents to the wind; an
	// hour-record position sits around 0.19 m². Below this the powered solver's
	// fixed speed bracket can fail to contain the true root at high power.
	minCdAM2 = 0.15
	maxCdAM2 = 2.0
	maxCrr   = 0.05
)

// Coefficients are the values the hybrid model can legitimately vary. The blend
// weight, drivetrain efficiency, air density and descent cap are versioned model
// constants in model.go, not operator inputs. They arrive as one file, loaded
// once at startup.
type Coefficients struct {
	CrrBySurface      map[surface.Kind]float64 // every surface.Kind mapped to the same scalar Crr — see crr's doc comment
	Fingerprint       string
	CalibrationCutoff string // "2025-08-01"; the last calibration ride's date, recorded as-is rather than computed from anything at load time
	MassKG            float64
	PowerWatts        float64
	CdAM2             float64
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

// crr returns the rolling resistance for a segment, selecting by kind with a
// KindUnknown-to-asphalt fallback. Load currently fills CrrBySurface with one
// scalar for every kind, so kind has no effect in practice; the lookup stays in
// case a future profile varies it again.
//
//nolint:gocritic // value receiver: Coefficients is immutable once loaded, and a pointer would let a caller mutate the shared instance mid-prediction.
func (c Coefficients) crr(kind surface.Kind) float64 {
	if kind == surface.KindUnknown {
		kind = surface.KindAsphalt
	}

	return c.CrrBySurface[kind]
}

// rawCoefficients is the TOML shape of the hybrid profile #239 emits.
type rawCoefficients struct {
	CalibrationCutoff string  `toml:"calibration_cutoff"`
	MassKG            float64 `toml:"mass_kg"`
	PowerWatts        float64 `toml:"power_watts"`
	CdAM2             float64 `toml:"cda_m2"`
	Crr               float64 `toml:"crr"`
	SecondsPerKM      float64 `toml:"seconds_per_km"`
	SecondsPerAscentM float64 `toml:"seconds_per_ascent_m"`
	EvaluatedRides    int     `toml:"evaluated_rides"`
	BiasPercent       float64 `toml:"bias_percent"`
	MAEPercent        float64 `toml:"mae_percent"`
	P90Percent        float64 `toml:"p90_percent"`

	TrainingWindowMonths int `toml:"training_window_months"`
}

// Load reads, parses, and validates a coefficient file. A missing, malformed, or
// physically impossible file is a startup failure. A file in the old
// physics-fitted schema fails here too: no compatibility path is carried.
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
	coefficients.Fingerprint = fingerprintOf(modelVersion, data)

	return coefficients, nil
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
	if r.MassKG < minMassKG || r.MassKG > maxMassKG {
		return Coefficients{}, fmt.Errorf("ridemodel: mass_kg must be between %g and %g", minMassKG, maxMassKG)
	}
	if r.PowerWatts < minPowerWatts || r.PowerWatts > maxPowerWatts {
		return Coefficients{}, fmt.Errorf("ridemodel: power_watts must be between %g and %g", minPowerWatts, maxPowerWatts)
	}
	if r.CdAM2 < minCdAM2 || r.CdAM2 > maxCdAM2 {
		return Coefficients{}, fmt.Errorf("ridemodel: cda_m2 must be between %g and %g", minCdAM2, maxCdAM2)
	}
	if r.Crr <= 0 || r.Crr > maxCrr {
		return Coefficients{}, fmt.Errorf("ridemodel: crr must be greater than 0 and at most %g", maxCrr)
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

	crr := map[surface.Kind]float64{
		surface.KindAsphalt:   r.Crr,
		surface.KindPaving:    r.Crr,
		surface.KindCompacted: r.Crr,
		surface.KindGravel:    r.Crr,
		surface.KindGround:    r.Crr,
	}

	return Coefficients{
		CalibrationCutoff: r.CalibrationCutoff,
		MassKG:            r.MassKG,
		PowerWatts:        r.PowerWatts,
		CdAM2:             r.CdAM2,
		SecondsPerKM:      r.SecondsPerKM,
		SecondsPerAscentM: r.SecondsPerAscentM,
		CrrBySurface:      crr,
		EvaluatedRides:    r.EvaluatedRides,
		BiasPercent:       r.BiasPercent,
		MAEPercent:        r.MAEPercent,
		P90Percent:        r.P90Percent,

		TrainingWindowMonths: r.TrainingWindowMonths,
	}, nil
}
