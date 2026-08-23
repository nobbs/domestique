package main

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syntheticRide builds one ride's worth of samples: a coasting section
// consistent with the given crr/cda at massKG, followed by a sustained climb
// at the given grade and speed — enough for both stage A and stage B to have
// something to fit from a single ride.
func syntheticRide(rideID string, date time.Time, crr, cda, massKG float64) []sampleRow {
	const airDensity = 1.2

	samples := make([]sampleRow, 0, 90)
	t := date
	speed := 15.0
	for range 60 { // 60 s coasting, downhill enough to sustain a realistic speed
		const grade = -3.0
		sinTheta := (grade / 100) / math.Sqrt(1+(grade/100)*(grade/100))
		cosTheta := 1 / math.Sqrt(1+(grade/100)*(grade/100))
		dissipative := crr*massKG*gravityMetresPerSecondSquared*cosTheta + cda*0.5*airDensity*speed*speed
		deltaV := (-dissipative/massKG - gravityMetresPerSecondSquared*sinTheta) * 1.0
		t = t.Add(time.Second)
		samples = append(samples, sampleRow{
			RideID: rideID, Time: t, DeltaSeconds: 1.0, IntervalDistance: speed, SpeedMPS: speed,
			GradientPercent: grade, HasAltitude: true, CadenceRPM: 0, HasCadence: true,
			Latitude: 50.0 + float64(len(samples))*0.00001, Longitude: 8.0, HasPosition: true, Moving: true,
		})
		speed += deltaV
	}

	climbSpeed := 4.0
	for range 30 { // 30 s of sustained climbing
		const grade = 6.0
		t = t.Add(time.Second)
		samples = append(samples, sampleRow{
			RideID: rideID, Time: t, DeltaSeconds: 1.0, IntervalDistance: climbSpeed, SpeedMPS: climbSpeed,
			GradientPercent: grade, HasAltitude: true, CadenceRPM: 80, HasCadence: true, Moving: true,
		})
	}

	return samples
}

func TestFitGroupRecoversKnownCoefficientsFromASyntheticCorpus(t *testing.T) {
	const massKG, crr, cda = 90.0, 0.006, 0.45
	start := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)

	samplesByRide := make(map[string][]sampleRow)
	var rides []rideRow
	for i := range 12 {
		rideID := "r" + string(rune('a'+i))
		date := start.AddDate(0, 0, i*7)
		samplesByRide[rideID] = syntheticRide(rideID, date, crr, cda, massKG)
		rides = append(rides, rideRow{RideID: rideID, Gear: "Bike A", Date: date, MovingSeconds: 90})
	}

	group := groupRidesByGear(rides)[0]
	require.False(t, group.Skipped)

	cfg := runConfig{massKG: massKG, driveEfficiency: 0.975, climbThresholdPercent: defaultClimbThresholdPercent}
	result := fitGroup(group, rides, samplesByRide, nil, &cfg)

	require.False(t, result.Skipped)
	require.False(t, result.RejectedCrrBounds)
	require.False(t, result.IllConditioned)
	assert.InDelta(t, crr, result.CrrOverall, 0.002)
	assert.InDelta(t, cda, result.CdA, 0.05)
	assert.Greater(t, result.PowerWatts, 0.0)
}

func TestFitGroupRejectsAPhysicallyImplausibleCrr(t *testing.T) {
	const massKG = 90.0
	start := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)

	samplesByRide := make(map[string][]sampleRow)
	var rides []rideRow
	for i := range 12 {
		rideID := "r" + string(rune('a'+i))
		date := start.AddDate(0, 0, i*7)
		// Fit against one crr/cda but corrupt every window's delta speed to
		// something a physically defensible fit cannot explain, without
		// tripping the plausibility filter itself (which is applied on a
		// per-window basis using the same box this fit will be judged
		// against) — a below-floor Crr comes from a fit whose regressors
		// are individually plausible but collectively pull the intercept
		// negative.
		samples := syntheticRide(rideID, date, 0.05, 2.0, massKG)
		samplesByRide[rideID] = samples
		rides = append(rides, rideRow{RideID: rideID, Gear: "Bike A", Date: date, MovingSeconds: 90})
	}

	group := groupRidesByGear(rides)[0]
	cfg := runConfig{massKG: massKG, driveEfficiency: 0.975, climbThresholdPercent: defaultClimbThresholdPercent, tyreCrrBench: 0.004, tyreCrrToleranceLow: 1.0, tyreCrrToleranceHigh: 1.5}
	result := fitGroup(group, rides, samplesByRide, nil, &cfg)

	assert.True(t, result.RejectedCrrBounds, "a Crr the configured tyre could not plausibly produce must fail, not be written")
}
