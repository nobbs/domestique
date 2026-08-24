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
calibration_cutoff = "2025-08-01"
mass_kg = 90.0
power_watts = 180.0
cda_m2 = 0.45
crr = 0.012
seconds_per_km = 145.3578
seconds_per_ascent_m = 3.2190
`

// oldFormatCoefficientsTOML is what internal/ridemodel loaded before #240:
// physical constants fitted from a ride corpus, including a [crr] table this
// package no longer reads as anything but a parse error.
const oldFormatCoefficientsTOML = `
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
	assert.InDelta(t, 180.0, coefficients.PowerWatts, 0, "power_watts")
	assert.InDelta(t, 0.45, coefficients.CdAM2, 0, "cda_m2")
	assert.InDelta(t, 145.3578, coefficients.SecondsPerKM, 0, "seconds_per_km")
	assert.InDelta(t, 3.2190, coefficients.SecondsPerAscentM, 0, "seconds_per_ascent_m")
	assert.Equal(t, "2025-08-01", coefficients.CalibrationCutoff, "calibration_cutoff")
	assert.InDelta(t, 0.012, coefficients.CrrBySurface[surface.KindAsphalt], 0, "crr applied to asphalt")
	assert.InDelta(t, 0.012, coefficients.CrrBySurface[surface.KindGround], 0, "crr applied uniformly to every surface")
	assert.NotEmpty(t, coefficients.Fingerprint, "fingerprint")
	assert.False(t, coefficients.HasValidation(), "a file with no measured benchmark result must not claim one")
}

// #217's four validation fields are optional, added after every field above
// was already required: a file written before they existed must still load.
func TestLoadAcceptsAFileWithoutValidationFields(t *testing.T) {
	coefficients, err := Load(writeCoefficientsFile(t, validCoefficientsTOML))
	require.NoError(t, err, "Load()")

	assert.False(t, coefficients.HasValidation(), "HasValidation() on a file with no evaluated_rides")
	assert.Zero(t, coefficients.EvaluatedRides, "EvaluatedRides")
	assert.Zero(t, coefficients.BiasPercent, "BiasPercent")
	assert.Zero(t, coefficients.MAEPercent, "MAEPercent")
	assert.Zero(t, coefficients.P90Percent, "P90Percent")
}

// A negative bias is a legitimate reading — the model can run fast as easily
// as slow — so it must load, not be rejected the way the physical fields
// above are bounds-checked.
func TestLoadAcceptsValidationFieldsIncludingANegativeBias(t *testing.T) {
	document := validCoefficientsTOML + `
evaluated_rides = 42
bias_percent = -1.20
mae_percent = 6.80
p90_percent = 14.10
`
	coefficients, err := Load(writeCoefficientsFile(t, document))
	require.NoError(t, err, "Load()")

	assert.True(t, coefficients.HasValidation(), "HasValidation() on a file that carries a measured result")
	assert.Equal(t, 42, coefficients.EvaluatedRides, "EvaluatedRides")
	assert.InDelta(t, -1.20, coefficients.BiasPercent, 0, "BiasPercent")
	assert.InDelta(t, 6.80, coefficients.MAEPercent, 0, "MAEPercent")
	assert.InDelta(t, 14.10, coefficients.P90Percent, 0, "P90Percent")
}

// A negative evaluated_rides, mae_percent, or p90_percent is not a
// physically meaningful reading — a count and two absolute-error
// magnitudes cannot go negative — so each is a startup failure rather than
// a value that silently loads and disables or corrupts HasValidation().
func TestLoadRejectsNegativeValidationFields(t *testing.T) {
	for name, addition := range map[string]string{
		"negative evaluated_rides": "evaluated_rides = -1\n",
		"negative mae_percent":     "evaluated_rides = 42\nmae_percent = -0.1\n",
		"negative p90_percent":     "evaluated_rides = 42\np90_percent = -0.1\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeCoefficientsFile(t, validCoefficientsTOML+addition))
			require.Error(t, err, "Load()")
		})
	}
}

func TestLoadFingerprintChangesWithFileContent(t *testing.T) {
	first, err := Load(writeCoefficientsFile(t, validCoefficientsTOML))
	require.NoError(t, err, "Load() first")
	second, err := Load(writeCoefficientsFile(t, strings.Replace(validCoefficientsTOML, "90.0", "91.0", 1)))
	require.NoError(t, err, "Load() second")

	assert.NotEqual(t, first.Fingerprint, second.Fingerprint, "changed content should change the fingerprint")
}

// TestFingerprintOfDoesNotCollideAcrossAVersionDataBoundaryShift is the exact
// case plain concatenation would get wrong: "ab"+"cdef" and "abcd"+"ef"
// concatenate to the same bytes, so a naive hash of version+data would give
// the same fingerprint to two different (version, file) pairs — defeating
// the guarantee that bumping modelVersion invalidates a cached prediction.
func TestFingerprintOfDoesNotCollideAcrossAVersionDataBoundaryShift(t *testing.T) {
	left := fingerprintOf("ab", []byte("cdef"))
	right := fingerprintOf("abcd", []byte("ef"))

	assert.NotEqual(t, left, right, "different (version, data) pairs must not share a fingerprint")
}

func TestLoadRejectsAMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.toml"))
	require.Error(t, err, "Load() on a missing file")
}

func TestLoadRejectsMalformedTOML(t *testing.T) {
	_, err := Load(writeCoefficientsFile(t, "this is not valid TOML {{{"))
	require.Error(t, err, "Load() on malformed TOML")
}

func TestLoadRejectsTheOldPhysicsFittedSchema(t *testing.T) {
	_, err := Load(writeCoefficientsFile(t, oldFormatCoefficientsTOML))
	require.Error(t, err, "Load() on the pre-#240 schema")
}

func TestLoadRejectsPhysicallyImplausibleValues(t *testing.T) {
	testCases := map[string]struct{ old, replacement string }{
		"mass too low":   {"mass_kg = 90.0", "mass_kg = 1.0"},
		"mass too high":  {"mass_kg = 90.0", "mass_kg = 1000.0"},
		"power too low":  {"power_watts = 180.0", "power_watts = 1.0"},
		"power too high": {"power_watts = 180.0", "power_watts = 5000.0"},
		"cda zero":       {"cda_m2 = 0.45", "cda_m2 = 0"},
		"cda too small to bound the powered solver": {"cda_m2 = 0.45", "cda_m2 = 0.05"},
		"cda absurd":                    {"cda_m2 = 0.45", "cda_m2 = 10.0"},
		"crr zero":                      {"crr = 0.012", "crr = 0"},
		"crr too high":                  {"crr = 0.012", "crr = 1.0"},
		"seconds per km zero":           {"seconds_per_km = 145.3578", "seconds_per_km = 0"},
		"seconds per km negative":       {"seconds_per_km = 145.3578", "seconds_per_km = -1.0"},
		"seconds per ascent metre zero": {"seconds_per_ascent_m = 3.2190", "seconds_per_ascent_m = 0"},
		"calibration cutoff not a date": {`calibration_cutoff = "2025-08-01"`, `calibration_cutoff = "not-a-date"`},
	}
	for name, testCase := range testCases {
		t.Run(name, func(t *testing.T) {
			document := strings.Replace(validCoefficientsTOML, testCase.old, testCase.replacement, 1)
			_, err := Load(writeCoefficientsFile(t, document))
			require.Error(t, err, "Load() should reject %s", name)
		})
	}
}
