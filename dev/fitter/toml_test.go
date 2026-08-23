package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/surface"
)

func TestWriteCoefficientsTOMLRoundTripsThroughTheSharedSchema(t *testing.T) {
	result := fitResult{
		CrrOverall: 0.006,
		CdA:        0.487,
		PowerWatts: 155.0,
		MassKG:     90.0,
		CrrBySurface: map[surface.Kind]float64{
			surface.KindGravel: 0.012,
		},
	}
	config := coefficientsConfig{
		DriveEfficiency:           0.975,
		AirDensityKGPerM3:         1.2,
		DescentCutoffPercent:      -1.0,
		DescentCapMetresPerSecond: 22.0,
	}
	path := filepath.Join(t.TempDir(), "coefficients-bike-a.toml")

	require.NoError(t, writeCoefficientsTOML(path, &result, config))

	var got rawCoefficients
	data, err := os.ReadFile(path) //nolint:gosec // path is this test's own t.TempDir() fixture, not external input
	require.NoError(t, err)
	require.NoError(t, toml.Unmarshal(data, &got))

	assert.InDelta(t, 90.0, got.MassKG, 1e-9)
	assert.InDelta(t, 155.0, got.PowerWatts, 1e-9)
	assert.InDelta(t, 0.975, got.DriveEfficiency, 1e-9)
	assert.InDelta(t, 0.487, got.CdAM2, 1e-9)
	assert.InDelta(t, 1.2, got.AirDensityKGPerM3, 1e-9)
	assert.InDelta(t, -1.0, got.DescentCutoffPercent, 1e-9)
	assert.InDelta(t, 22.0, got.DescentCapMetresPerSecond, 1e-9)
	// Gravel was fitted directly; every other class falls back to the
	// group's pooled Crr rather than being left at zero, which the real
	// loader would reject.
	assert.InDelta(t, 0.012, got.Crr.Gravel, 1e-9)
	assert.InDelta(t, 0.006, got.Crr.Asphalt, 1e-9)
	assert.InDelta(t, 0.006, got.Crr.Paving, 1e-9)
	assert.InDelta(t, 0.006, got.Crr.Compacted, 1e-9)
	assert.InDelta(t, 0.006, got.Crr.Ground, 1e-9)
}

func TestCrrForSurfaceFallsBackToOverallWhenAClassWasNeverLabelled(t *testing.T) {
	result := fitResult{CrrOverall: 0.007, CrrBySurface: map[surface.Kind]float64{}}
	assert.InDelta(t, 0.007, crrForSurface(&result, surface.KindAsphalt), 1e-9)
}
