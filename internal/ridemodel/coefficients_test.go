package ridemodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/surface"
)

const validCoefficientsTOML = `
mass_kg = 90.0
power_watts = 155.0
drive_efficiency = 0.975
cda_m2 = 0.487
air_density_kg_per_m3 = 1.2
descent_cutoff_percent = -1.0
descent_cap_metres_per_second = 22.0

[crr]
asphalt = 0.005
paving = 0.006
compacted = 0.007
gravel = 0.009
ground = 0.011
`

func writeCoefficientsFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ridemodel.toml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600), "writing coefficient file")

	return path
}

func TestLoadAcceptsAValidFile(t *testing.T) {
	path := writeCoefficientsFile(t, validCoefficientsTOML)

	coefficients, err := Load(path)
	require.NoError(t, err, "Load()")
	assert.InDelta(t, 90.0, coefficients.MassKG, 0, "mass_kg")
	assert.InDelta(t, 155.0, coefficients.PowerWatts, 0, "power_watts")
	assert.InDelta(t, 0.005, coefficients.CrrBySurface[surface.KindAsphalt], 0, "crr.asphalt")
	assert.NotEmpty(t, coefficients.Fingerprint, "fingerprint")
}

func TestLoadFingerprintChangesWithFileContent(t *testing.T) {
	first, err := Load(writeCoefficientsFile(t, validCoefficientsTOML))
	require.NoError(t, err, "Load() first")
	second, err := Load(writeCoefficientsFile(t, strings.Replace(validCoefficientsTOML, "90.0", "91.0", 1)))
	require.NoError(t, err, "Load() second")

	assert.NotEqual(t, first.Fingerprint, second.Fingerprint, "changed content should change the fingerprint")
}

func TestLoadRejectsAMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	require.Error(t, err, "Load() on a missing file")
}

func TestLoadRejectsMalformedTOML(t *testing.T) {
	_, err := Load(writeCoefficientsFile(t, "this is not valid TOML {{{"))
	require.Error(t, err, "Load() on malformed TOML")
}

func TestLoadRejectsPhysicallyImplausibleValues(t *testing.T) {
	testCases := map[string]struct{ old, replacement string }{
		"mass too low":            {"mass_kg = 90.0", "mass_kg = 1.0"},
		"mass too high":           {"mass_kg = 90.0", "mass_kg = 1000.0"},
		"power too low":           {"power_watts = 155.0", "power_watts = 1.0"},
		"power too high":          {"power_watts = 155.0", "power_watts = 5000.0"},
		"efficiency zero":         {"drive_efficiency = 0.975", "drive_efficiency = 0"},
		"efficiency over one":     {"drive_efficiency = 0.975", "drive_efficiency = 1.5"},
		"cda zero":                {"cda_m2 = 0.487", "cda_m2 = 0"},
		"cda absurd":              {"cda_m2 = 0.487", "cda_m2 = 10.0"},
		"air density too low":     {"air_density_kg_per_m3 = 1.2", "air_density_kg_per_m3 = 0.1"},
		"air density too high":    {"air_density_kg_per_m3 = 1.2", "air_density_kg_per_m3 = 5.0"},
		"positive descent cutoff": {"descent_cutoff_percent = -1.0", "descent_cutoff_percent = 1.0"},
		"descent cap zero":        {"descent_cap_metres_per_second = 22.0", "descent_cap_metres_per_second = 0"},
		"descent cap absurd":      {"descent_cap_metres_per_second = 22.0", "descent_cap_metres_per_second = 500.0"},
		"crr asphalt zero":        {"asphalt = 0.005", "asphalt = 0"},
		"crr gravel too high":     {"gravel = 0.009", "gravel = 1.0"},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			document := strings.Replace(validCoefficientsTOML, testCase.old, testCase.replacement, 1)
			_, err := Load(writeCoefficientsFile(t, document))
			require.Error(t, err, "Load() should reject %s", name)
		})
	}
}
