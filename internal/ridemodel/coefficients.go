// Package ridemodel is the forward physical model that turns a stage's
// geometry and a set of fitted coefficients into a predicted moving time.
//
// It is a pure function of its inputs: nothing here reaches a network, SQLite,
// or the clock. dev/ridemodel's fitter and this package's Predictor both run
// exactly this model, so a coefficient set means the same thing wherever it is
// evaluated.
package ridemodel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"

	"github.com/nobbs/domestique/internal/surface"
)

// Physical plausibility bounds for a loaded coefficient file. They are wide
// enough to admit any rider and machine and narrow enough to catch a
// transposed unit or a fit that did not converge — the same purpose #215's own
// tyre-relative Crr band serves, drawn here instead against the file this
// service actually loads.
const (
	minMassKG        = 20.0
	maxMassKG        = 300.0
	minPowerWatts    = 20.0
	maxPowerWatts    = 1000.0
	maxCdAM2         = 2.0
	minAirDensity    = 0.8
	maxAirDensity    = 1.5
	maxCrr           = 0.05
	maxDescentCapMPS = 40.0 // 144 km/h; a safety ceiling, not a claim anyone rides it.
)

// Coefficients are the fitted and configured constants the forward model needs
// to turn geometry into time. They arrive as one file, loaded once at startup.
type Coefficients struct {
	CrrBySurface              map[surface.Kind]float64
	Fingerprint               string
	MassKG                    float64
	PowerWatts                float64
	DriveEfficiency           float64
	CdAM2                     float64
	AirDensityKGPerM3         float64
	DescentCutoffPercent      float64
	DescentCapMetresPerSecond float64
}

// crr returns the rolling resistance for a surface class. KindUnknown takes
// asphalt's value: it means nobody has surveyed that stretch, and treating
// unsurveyed ground as rough would penalise a stage by how well mapped it is
// rather than by how it rides — the same reasoning the repository already
// applies to drawing unclassified stages plainly.
//
//nolint:gocritic // value receiver: Coefficients is immutable once loaded, and a pointer would let a caller mutate the shared instance mid-prediction.
func (c Coefficients) crr(kind surface.Kind) float64 {
	if kind == surface.KindUnknown {
		kind = surface.KindAsphalt
	}

	return c.CrrBySurface[kind]
}

// rawCoefficients is the TOML shape of the coefficient file #215 emits.
type rawCoefficients struct {
	Crr                       rawCrr  `toml:"crr"`
	MassKG                    float64 `toml:"mass_kg"`
	PowerWatts                float64 `toml:"power_watts"`
	DriveEfficiency           float64 `toml:"drive_efficiency"`
	CdAM2                     float64 `toml:"cda_m2"`
	AirDensityKGPerM3         float64 `toml:"air_density_kg_per_m3"`
	DescentCutoffPercent      float64 `toml:"descent_cutoff_percent"`
	DescentCapMetresPerSecond float64 `toml:"descent_cap_metres_per_second"`
}

// rawCrr carries rolling resistance per surface class named in the repository's
// own vocabulary (internal/surface.Kind.String), rather than the raw
// OpenStreetMap tags that vocabulary already collapses.
type rawCrr struct {
	Asphalt   float64 `toml:"asphalt"`
	Paving    float64 `toml:"paving"`
	Compacted float64 `toml:"compacted"`
	Gravel    float64 `toml:"gravel"`
	Ground    float64 `toml:"ground"`
}

// Load reads, parses, and validates a coefficient file. A missing, malformed,
// or physically impossible file is a startup failure: main refuses to serve a
// prediction it cannot stand behind rather than falling back to silence.
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

	sum := sha256.Sum256(data)
	coefficients.Fingerprint = hex.EncodeToString(sum[:])

	return coefficients, nil
}

//nolint:gocritic // value receiver: rawCoefficients is a short-lived TOML decode target, not a hot path.
func (r rawCoefficients) build() (Coefficients, error) {
	if r.MassKG < minMassKG || r.MassKG > maxMassKG {
		return Coefficients{}, fmt.Errorf("ridemodel: mass_kg must be between %g and %g", minMassKG, maxMassKG)
	}
	if r.PowerWatts < minPowerWatts || r.PowerWatts > maxPowerWatts {
		return Coefficients{}, fmt.Errorf("ridemodel: power_watts must be between %g and %g", minPowerWatts, maxPowerWatts)
	}
	if r.DriveEfficiency <= 0 || r.DriveEfficiency > 1 {
		return Coefficients{}, errors.New("ridemodel: drive_efficiency must be greater than 0 and at most 1")
	}
	if r.CdAM2 <= 0 || r.CdAM2 > maxCdAM2 {
		return Coefficients{}, fmt.Errorf("ridemodel: cda_m2 must be greater than 0 and at most %g", maxCdAM2)
	}
	if r.AirDensityKGPerM3 < minAirDensity || r.AirDensityKGPerM3 > maxAirDensity {
		return Coefficients{}, fmt.Errorf(
			"ridemodel: air_density_kg_per_m3 must be between %g and %g", minAirDensity, maxAirDensity,
		)
	}
	if r.DescentCutoffPercent > 0 {
		return Coefficients{}, errors.New("ridemodel: descent_cutoff_percent must not be positive")
	}
	if r.DescentCapMetresPerSecond <= 0 || r.DescentCapMetresPerSecond > maxDescentCapMPS {
		return Coefficients{}, fmt.Errorf(
			"ridemodel: descent_cap_metres_per_second must be greater than 0 and at most %g", maxDescentCapMPS,
		)
	}

	crr := map[surface.Kind]float64{
		surface.KindAsphalt:   r.Crr.Asphalt,
		surface.KindPaving:    r.Crr.Paving,
		surface.KindCompacted: r.Crr.Compacted,
		surface.KindGravel:    r.Crr.Gravel,
		surface.KindGround:    r.Crr.Ground,
	}
	for _, kind := range []surface.Kind{
		surface.KindAsphalt, surface.KindPaving, surface.KindCompacted, surface.KindGravel, surface.KindGround,
	} {
		if value := crr[kind]; value <= 0 || value > maxCrr {
			return Coefficients{}, fmt.Errorf("ridemodel: crr.%s must be greater than 0 and at most %g", kind, maxCrr)
		}
	}

	return Coefficients{
		MassKG:                    r.MassKG,
		PowerWatts:                r.PowerWatts,
		DriveEfficiency:           r.DriveEfficiency,
		CdAM2:                     r.CdAM2,
		AirDensityKGPerM3:         r.AirDensityKGPerM3,
		DescentCutoffPercent:      r.DescentCutoffPercent,
		DescentCapMetresPerSecond: r.DescentCapMetresPerSecond,
		CrrBySurface:              crr,
	}, nil
}
