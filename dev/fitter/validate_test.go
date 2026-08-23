package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplitByDateNeverShufflesAcrossTheBoundary(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	rides := make([]rideRow, 20)
	for i := range rides {
		rides[i] = rideRow{RideID: string(rune('a' + i)), Date: start.AddDate(0, 0, i)}
	}

	train, heldOut := splitByDate(rides)
	require.NotEmpty(t, heldOut)
	for _, tr := range train {
		for _, ho := range heldOut {
			assert.True(t, tr.Date.Before(ho.Date), "a training ride must not be later than a held-out one")
		}
	}
}

func TestPredictedSpeedRecoversTheConfiguredPowerOnAClimb(t *testing.T) {
	const crr, cda, massKG, efficiency, powerWatts = 0.006, 0.45, 90.0, 0.975, 155.0

	v := predictedSpeed(6.0, 1.2, crr, cda, massKG, efficiency, powerWatts, -1.0, 22.0)
	require.Greater(t, v, 0.0)

	gotPower := climbPowerWatts(climbSample{MeanSpeedMPS: v, GradePercent: 6.0, AirDensity: 1.2}, crr, cda, massKG, efficiency)
	assert.InDelta(t, powerWatts, gotPower, 0.1)
}

func TestPredictedSpeedCapsADescentRatherThanSolvingForIt(t *testing.T) {
	v := predictedSpeed(-5.0, 1.2, 0.006, 0.45, 90.0, 0.975, 155.0, -1.0, 22.0)
	assert.InDelta(t, 22.0, v, 1e-9)
}

func TestValidateHeldOutReportsBothMAEsOverTheHeldOutRidesOnly(t *testing.T) {
	when := time.Date(2026, 6, 1, 6, 0, 0, 0, time.UTC)
	samplesByRide := map[string][]sampleRow{
		"heldout1": {
			{RideID: "heldout1", Time: when, DeltaSeconds: 100, IntervalDistance: 400, Moving: true, HasAltitude: true, GradientPercent: 6.0},
		},
	}
	result := fitResult{CrrOverall: 0.006, CdA: 0.45, MassKG: 90.0, PowerWatts: 155.0}
	config := coefficientsConfig{DriveEfficiency: 0.975, DescentCutoffPercent: -1.0, DescentCapMetresPerSecond: 22.0}

	summary := validateHeldOut(
		map[string]bool{"heldout1": true}, samplesByRide, map[string]float64{"heldout1": 100.0},
		&result, config, 8.0, 500.0,
	)
	assert.Equal(t, 1, summary.Rides)
	assert.GreaterOrEqual(t, summary.ModelMAE, 0.0)
	assert.GreaterOrEqual(t, summary.BaselineMAE, 0.0)
}
