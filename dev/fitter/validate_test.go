package main

import (
	"math"
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

func TestPredictedSpeedSolvesTheCoastEquilibriumOnAModerateDescent(t *testing.T) {
	const crr, cda, massKG = 0.006, 0.45, 90.0

	v := predictedSpeed(-5.0, 1.2, crr, cda, massKG, 0.975, 155.0, -1.0, 22.0)
	require.Greater(t, v, 0.0)
	require.Less(t, v, 22.0, "a moderate descent must not jump straight to the cap")

	// At the solved speed, drag and rolling resistance exactly balance
	// gravity's forward push — net force is zero.
	dissipative := crr*massKG*gravityMetresPerSecondSquared*(1/math.Sqrt(1+0.05*0.05)) + 0.5*1.2*cda*v*v
	drivingForce := massKG * gravityMetresPerSecondSquared * (0.05 / math.Sqrt(1+0.05*0.05))
	assert.InDelta(t, drivingForce, dissipative, 0.5)
}

func TestPredictedSpeedCapsAVerySteepDescentAtTheConfiguredCap(t *testing.T) {
	v := predictedSpeed(-25.0, 1.2, 0.006, 0.45, 90.0, 0.975, 155.0, -1.0, 22.0)
	assert.InDelta(t, 22.0, v, 1e-9)
}

func TestPredictedSpeedReturnsZeroWhenAGradeJustPastTheCutoffCannotSustainACoast(t *testing.T) {
	v := predictedSpeed(-1.01, 1.2, 0.02, 0.45, 90.0, 0.975, 155.0, -1.0, 22.0)
	assert.InDelta(t, 0.0, v, 1e-9)
}

func TestPredictedSpeedReturnsZeroWhenTheConfiguredPowerExceedsWhatTheBracketCanReach(t *testing.T) {
	v := predictedSpeed(20.0, 1.2, 0.006, 0.45, 90.0, 0.975, 100000.0, -1.0, 22.0)
	assert.InDelta(t, 0.0, v, 1e-9)
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
		&result, config, 8.0, 500.0, defaultClimbThresholdPercent,
	)
	assert.Equal(t, 1, summary.Rides)
	assert.GreaterOrEqual(t, summary.ModelMAE, 0.0)
	assert.GreaterOrEqual(t, summary.BaselineMAE, 0.0)
}

// A run configured with a non-default -climb-threshold-percent must have
// its baseline split flat from climbing at that same threshold, not the
// package's own default — otherwise the baseline the model is compared
// against does not describe the run it actually did.
func TestBaselineMovingSecondsUsesTheConfiguredClimbThreshold(t *testing.T) {
	samples := []sampleRow{
		{Moving: true, HasAltitude: true, IntervalDistance: 100, DeltaSeconds: 10, GradientPercent: 3.0},
	}

	// Below a 2% threshold this sample counts as climbing (ascent), so the
	// flat-speed term contributes nothing; above a 5% threshold it counts as
	// flat, so the VAM term contributes nothing.
	asClimb := baselineMovingSeconds(samples, 8.0, 500.0, 2.0)
	asFlat := baselineMovingSeconds(samples, 8.0, 500.0, 5.0)
	assert.NotEqual(t, asClimb, asFlat)
	assert.InDelta(t, 100.0/8.0, asFlat, 1e-9)
}
