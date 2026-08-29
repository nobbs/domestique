package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig is a runConfig that passes validate(), for tests that mutate
// exactly one field to check it is rejected.
func validConfig() runConfig {
	return runConfig{
		corpusDir: "corpus", coefficientsPath: "ridemodel.toml",
		etaRouteCellDegrees: defaultRouteCellDegrees, etaRouteJaccard: defaultRouteJaccardThreshold,
		etaTrainingMonths: defaultTrainingWindowMonths,
	}
}

func TestRunConfigValidateAcceptsAWellFormedConfig(t *testing.T) {
	cfg := validConfig()
	assert.NoError(t, cfg.validate())
}

func TestRunConfigValidateRejectsEachInvalidFlagCombination(t *testing.T) {
	for name, mutate := range map[string]func(*runConfig){
		"missing corpus":             func(c *runConfig) { c.corpusDir = "" },
		"missing coefficients":       func(c *runConfig) { c.coefficientsPath = "" },
		"non-positive route cell":    func(c *runConfig) { c.etaRouteCellDegrees = 0 },
		"route overlap above one":    func(c *runConfig) { c.etaRouteJaccard = 1.1 },
		"route overlap zero":         func(c *runConfig) { c.etaRouteJaccard = 0 },
		"training window zero":       func(c *runConfig) { c.etaTrainingMonths = 0 },
		"training window below zero": func(c *runConfig) { c.etaTrainingMonths = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			mutate(&cfg)
			assert.Error(t, cfg.validate())
		})
	}
}

const testCoefficientsTOML = `
calibration_cutoff = "2024-01-10"
mass_kg = 90.0
power_watts = 180.0
cda_m2 = 0.45
crr = 0.012
seconds_per_km = 150.0
seconds_per_ascent_m = 4.0
`

// writeCorpus writes samples.csv and rides.csv in dev/ridemodel's own
// column order, from in-memory rows — a small, self-contained CSV encoder
// rather than a dependency on dev/ridemodel's own writer, which lives in a
// different command and a different package.
func writeCorpus(t *testing.T, dir string, rides []rideRow, samplesByRide map[string][]sampleRow) {
	t.Helper()

	var samplesCSV strings.Builder
	samplesCSV.WriteString("ride_id,time,delta_seconds,interval_distance_m,speed_mps,gradient_percent,altitude_m,has_altitude,latitude,longitude,has_position,moving\n")
	for _, ride := range rides {
		for _, s := range samplesByRide[ride.RideID] {
			fmt.Fprintf(&samplesCSV, "%s,%s,%v,%v,%v,%v,%v,%v,%v,%v,%v,%v\n",
				ride.RideID, s.Time.Format(time.RFC3339), s.DeltaSeconds, s.IntervalDistance, s.SpeedMPS,
				s.GradientPercent, s.AltitudeM, s.HasAltitude, s.Latitude, s.Longitude, s.HasPosition, s.Moving)
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "samples.csv"), []byte(samplesCSV.String()), 0o600))

	var ridesCSV strings.Builder
	ridesCSV.WriteString("ride_id,date,gear,moving_seconds\n")
	for _, ride := range rides {
		fmt.Fprintf(&ridesCSV, "%s,%s,%s,%v\n", ride.RideID, ride.Date.Format(time.RFC3339), ride.Gear, ride.MovingSeconds)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rides.csv"), []byte(ridesCSV.String()), 0o600))
}

func TestRunEvaluatesTheLoadedProfileEndToEnd(t *testing.T) {
	rides, samplesByRide := syntheticCorpus(30)
	// syntheticCorpus sets each ride's MovingSeconds from its own distance and
	// ascent; spread that total evenly across the sample's points rather than the
	// placeholder one-second spacing, so the moving-time hygiene check and the
	// model's predictions are consistent with each other.
	for i := range rides {
		samples := samplesByRide[rides[i].RideID]
		perSample := rides[i].MovingSeconds / float64(len(samples))
		clock := rides[i].Date
		for j := range samples {
			samples[j].DeltaSeconds = perSample
			samples[j].SpeedMPS = samples[j].IntervalDistance / perSample
			clock = clock.Add(time.Duration(perSample * float64(time.Second)))
			samples[j].Time = clock
		}
	}

	dir := t.TempDir()
	writeCorpus(t, dir, rides, samplesByRide)
	coefficientsPath := filepath.Join(dir, "ridemodel.toml")
	require.NoError(t, os.WriteFile(coefficientsPath, []byte(testCoefficientsTOML), 0o600))

	cfg := &runConfig{
		corpusDir: dir, coefficientsPath: coefficientsPath,
		etaRouteCellDegrees: defaultRouteCellDegrees, etaRouteJaccard: defaultRouteJaccardThreshold,
		etaTrainingMonths: defaultTrainingWindowMonths,
	}
	err := run(cfg)
	require.NoError(t, err)
}

func TestRunFailsClearlyOnAMissingCoefficientsFile(t *testing.T) {
	dir := t.TempDir()
	rides, samplesByRide := syntheticCorpus(15)
	writeCorpus(t, dir, rides, samplesByRide)

	cfg := &runConfig{
		corpusDir: dir, coefficientsPath: filepath.Join(dir, "missing.toml"),
		etaRouteCellDegrees: defaultRouteCellDegrees, etaRouteJaccard: defaultRouteJaccardThreshold,
		etaTrainingMonths: defaultTrainingWindowMonths,
	}
	err := run(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading coefficients")
}
