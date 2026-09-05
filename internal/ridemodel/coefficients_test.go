package ridemodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validCoefficientsTOML = `
calibration_cutoff = "2025-08-01"
seconds_per_km = 145.3578
seconds_per_ascent_m = 3.2190
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
	assert.InDelta(t, 145.3578, coefficients.SecondsPerKM, 0, "seconds_per_km")
	assert.InDelta(t, 3.2190, coefficients.SecondsPerAscentM, 0, "seconds_per_ascent_m")
	assert.Equal(t, "2025-08-01", coefficients.CalibrationCutoff, "calibration_cutoff")
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
// as slow — so it must load rather than be rejected as out of range.
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

// A negative evaluated_rides, mae_percent, p90_percent, or
// training_window_months is not a meaningful reading — a count,
// two absolute-error magnitudes and a span of months cannot go negative — so each is a startup failure rather than
// a value that silently loads and disables or corrupts HasValidation().
func TestLoadRejectsNegativeValidationFields(t *testing.T) {
	for name, addition := range map[string]string{
		"negative evaluated_rides": "evaluated_rides = -1\n",
		"negative mae_percent":     "evaluated_rides = 42\nmae_percent = -0.1\n",
		"negative p90_percent":     "evaluated_rides = 42\np90_percent = -0.1\n",
		"negative training window": "training_window_months = -1\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeCoefficientsFile(t, validCoefficientsTOML+addition))
			require.Error(t, err, "Load()")
		})
	}
}

// A percentage set without evaluated_rides is a partially-updated file, not
// an intentionally absent group: loading it would silently disable
// HasValidation() and drop the metadata the operator meant to add.
func TestLoadRejectsAValidationPercentageWithoutEvaluatedRides(t *testing.T) {
	for name, addition := range map[string]string{
		"bias_percent alone": "bias_percent = -1.2\n",
		"mae_percent alone":  "mae_percent = 6.8\n",
		"p90_percent alone":  "p90_percent = 14.1\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeCoefficientsFile(t, validCoefficientsTOML+addition))
			require.Error(t, err, "Load()")
		})
	}
}

// training_window_months records how far back the fit that produced this
// profile reached, so the profile can be reproduced from its own metadata.
// It is optional: a file written before #251 omits it and still loads.
func TestLoadAcceptsAndReportsTheTrainingWindow(t *testing.T) {
	withWindow, err := Load(writeCoefficientsFile(t, validCoefficientsTOML+"training_window_months = 12\n"))
	require.NoError(t, err, "Load()")
	assert.Equal(t, 12, withWindow.TrainingWindowMonths)

	without, err := Load(writeCoefficientsFile(t, validCoefficientsTOML))
	require.NoError(t, err, "Load()")
	assert.Equal(t, 0, without.TrainingWindowMonths, "a file predating the field must load with no window recorded")
}

func TestLoadFingerprintChangesWithFileContent(t *testing.T) {
	first, err := Load(writeCoefficientsFile(t, validCoefficientsTOML))
	require.NoError(t, err, "Load() first")
	second, err := Load(writeCoefficientsFile(t, strings.Replace(validCoefficientsTOML, "145.3578", "146.0", 1)))
	require.NoError(t, err, "Load() second")

	assert.NotEqual(t, first.Fingerprint, second.Fingerprint, "changed content should change the fingerprint")
}

// The exact case plain concatenation would get wrong: "ab"+"cdef" and
// "abcd"+"ef" concatenate to the same bytes, so a naive hash of version+data
// would give the same fingerprint to two different (version, file) pairs.
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

// A file written for the hybrid model still names the physics inputs it fitted.
// Those keys are no longer read, and their presence must not fail the load.
func TestLoadIgnoresTheRetiredPhysicsKeys(t *testing.T) {
	document := validCoefficientsTOML + `
mass_kg = 90.0
power_watts = 180.0
cda_m2 = 0.45
crr = 0.012
`
	coefficients, err := Load(writeCoefficientsFile(t, document))
	require.NoError(t, err, "Load()")
	assert.InDelta(t, 145.3578, coefficients.SecondsPerKM, 0, "seconds_per_km")
}

func TestLoadRejectsImplausibleValues(t *testing.T) {
	testCases := map[string]struct{ old, replacement string }{
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

func TestLoadRejectsANonFiniteTerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ridemodel.toml")
	require.NoError(t, os.WriteFile(path, []byte("seconds_per_km = inf\nseconds_per_ascent_m = 3.2\n"), 0o600))

	_, err := Load(path)
	require.ErrorContains(t, err, "finite")
}
