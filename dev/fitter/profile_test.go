package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/ridemodel"
)

// The copy-ready profile is pasted verbatim into ridemodel.toml, so whatever
// it prints has to be a file internal/ridemodel.Load actually accepts.
func TestPrintCopyReadyProfileEmitsALoadableFile(t *testing.T) {
	rides, samplesByRide := monthlySyntheticCorpus(60)
	coefficients := testCoefficients()
	cfg := recalibrateConfig()
	clusters, _, _, _ := clusterRoutes(rides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	eval, err := runRecalibration(rides, samplesByRide, clusters, &coefficients, cfg)
	require.NoError(t, err)

	var report strings.Builder
	printCopyReadyProfile(&report, &eval)

	var profile strings.Builder
	for line := range strings.SplitSeq(report.String(), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "=") && !strings.HasPrefix(trimmed, "validation") {
			fmt.Fprintln(&profile, trimmed)
		}
	}

	path := filepath.Join(t.TempDir(), "ridemodel.toml")
	require.NoError(t, os.WriteFile(path, []byte(profile.String()), 0o600))

	loaded, err := ridemodel.Load(path)
	require.NoError(t, err, "the printed profile must load:\n%s", profile.String())
	assert.InDelta(t, eval.secondsPerKM, loaded.SecondsPerKM, 1e-4)
	assert.Equal(t, cfg.etaTrainingMonths, loaded.TrainingWindowMonths)
}
