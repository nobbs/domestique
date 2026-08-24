// Package ridemodel is the forward physical model that turns a stage's
// geometry and a set of hybrid coefficients into a predicted moving time.
//
// It is a pure function of its inputs: nothing here reaches a network, SQLite,
// or the clock. dev/fitter's benchmark and this package's Predictor both run
// exactly this model, so a coefficient set means the same thing wherever it is
// evaluated.
package ridemodel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/nobbs/domestique/internal/surface"
)

// Physical plausibility bounds for a loaded coefficient file. They are wide
// enough to admit any rider and machine and narrow enough to catch a
// transposed unit or a fit that did not converge — the same purpose #215's own
// tyre-relative Crr band serves, drawn here instead against the file this
// service actually loads.
const (
	minMassKG     = 20.0
	maxMassKG     = 300.0
	minPowerWatts = 20.0
	maxPowerWatts = 1000.0
	// minCdAM2 is below anything a person on a bicycle presents to the wind —
	// an aggressive hour-record position sits around 0.19 m². Below this, the
	// powered solver's fixed speed bracket ([0, maxSolveSpeedMetresPerSecond])
	// can fail to contain the equation's true root at high configured power,
	// which would otherwise report a crawl speed rather than a fast one.
	minCdAM2 = 0.15
	maxCdAM2 = 2.0
	maxCrr   = 0.05
)

// Coefficients are the values the accepted hybrid model — #213's equal-weight
// average of fixed physics and a rides-calibrated route correction — can
// legitimately vary. Everything else (the 50/50 weight, drivetrain efficiency,
// standard air density, the descent cutoff and cap) is a versioned model
// constant in model.go, not an operator input; see modelVersion's own comment
// for why that split is what makes an upgrade to those constants still
// invalidate a cached prediction. They arrive as one file, loaded once at
// startup.
type Coefficients struct {
	CrrBySurface      map[surface.Kind]float64 // every surface.Kind mapped to the same scalar Crr — see crr's doc comment
	Fingerprint       string
	CalibrationCutoff string // "2025-08-01"; the last calibration ride's date, recorded rather than computed from
	MassKG            float64
	PowerWatts        float64
	CdAM2             float64
	SecondsPerKM      float64
	SecondsPerAscentM float64
}

// crr returns the rolling resistance for a segment. It ignores kind: #239's
// route-disjoint benchmark found the per-surface Crr table no material
// improvement over one scalar value on the operator's real corpus, so Load
// fills CrrBySurface with that same scalar for every surface.Kind rather than
// varying it. The parameter and the KindUnknown-to-asphalt fallback the
// surface classification pipeline still expects both stay in the signature —
// Predict, Predictor, and the cache's surface-generation tracking are
// unchanged by this — but every kind now reads the same value.
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
}

// Load reads, parses, and validates a coefficient file. A missing, malformed,
// or physically impossible file is a startup failure: main refuses to serve a
// prediction it cannot stand behind rather than falling back to silence. A
// file in the old physics-fitted schema — a `[crr]` table, `drive_efficiency`,
// no `seconds_per_km` — fails here too: `crr` no longer unmarshals as a table
// into a float field, and the new required fields are absent, so it is
// rejected the same way any other malformed file is, with no compatibility
// path carried for a schema this service no longer runs.
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

	// modelVersion is mixed into the hash, not just the file's own bytes: a
	// code upgrade that changes one of model.go's versioned constants must
	// still invalidate a cached prediction even when the operator's file is
	// byte-for-byte unchanged. See modelVersion's own comment.
	hash := sha256.New()
	hash.Write([]byte(modelVersion))
	hash.Write(data)
	coefficients.Fingerprint = hex.EncodeToString(hash.Sum(nil))

	return coefficients, nil
}

func (r rawCoefficients) build() (Coefficients, error) {
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
	}, nil
}
