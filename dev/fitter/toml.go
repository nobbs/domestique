package main

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"

	"github.com/nobbs/domestique/internal/surface"
)

// rawCoefficients is the exact TOML shape internal/ridemodel/coefficients.go
// (#216, PR #232) unmarshals — this package's contract with the forward
// model, not something this package is free to redesign. Field names and the
// [crr] sub-table match that loader's rawCoefficients byte for byte.
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

type rawCrr struct {
	Asphalt   float64 `toml:"asphalt"`
	Paving    float64 `toml:"paving"`
	Compacted float64 `toml:"compacted"`
	Gravel    float64 `toml:"gravel"`
	Ground    float64 `toml:"ground"`
}

// coefficientsConfig carries the operator-supplied constants the issue scopes
// as inputs rather than fits: mass is weighed, not estimated; the rest are
// deployment defaults matching #216's own example configuration.
type coefficientsConfig struct {
	DriveEfficiency           float64
	AirDensityKGPerM3         float64
	DescentCutoffPercent      float64
	DescentCapMetresPerSecond float64
}

// crrForSurface reads a group's fitted per-surface table, falling back to the
// overall pooled Crr for a class with too few samples to fit its own —
// internal/ridemodel's loader requires every one of the five keys to be
// present and positive, so a class this run never labelled still needs a
// physically defensible value rather than a zero that would fail to load.
func crrForSurface(result *fitResult, kind surface.Kind) float64 {
	if v, ok := result.CrrBySurface[kind]; ok && v > 0 {
		return v
	}

	return result.CrrOverall
}

func writeCoefficientsTOML(path string, result *fitResult, config coefficientsConfig) error {
	raw := rawCoefficients{
		MassKG:                    result.MassKG,
		PowerWatts:                result.PowerWatts,
		DriveEfficiency:           config.DriveEfficiency,
		CdAM2:                     result.CdA,
		AirDensityKGPerM3:         config.AirDensityKGPerM3,
		DescentCutoffPercent:      config.DescentCutoffPercent,
		DescentCapMetresPerSecond: config.DescentCapMetresPerSecond,
		Crr: rawCrr{
			Asphalt:   crrForSurface(result, surface.KindAsphalt),
			Paving:    crrForSurface(result, surface.KindPaving),
			Compacted: crrForSurface(result, surface.KindCompacted),
			Gravel:    crrForSurface(result, surface.KindGravel),
			Ground:    crrForSurface(result, surface.KindGround),
		},
	}

	data, err := toml.Marshal(raw)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}

	// The written file carries the mass and coefficients this group was
	// fitted against, so it is written readable to the operator alone —
	// dev/ridemodel's own output CSVs follow the same reasoning.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
