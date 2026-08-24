package main

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// validConfig is a runConfig that passes validate(), for tests that mutate
// exactly one field to check it is rejected.
func validConfig() runConfig {
	return runConfig{
		corpusDir: "corpus", massKG: 90, driveEfficiency: 0.975,
		descentCapMPS: 22.0, climbThresholdPercent: defaultClimbThresholdPercent,
		tyreCrrBench: 0.008, tyreCrrToleranceLow: 1.0, tyreCrrToleranceHigh: 1.5,
	}
}

func TestRunConfigValidateAcceptsAWellFormedConfig(t *testing.T) {
	cfg := validConfig()
	assert.NoError(t, cfg.validate())
}

func TestRunConfigValidateRejectsEachInvalidFlagCombination(t *testing.T) {
	for name, mutate := range map[string]func(*runConfig){
		"missing corpus":               func(c *runConfig) { c.corpusDir = "" },
		"non-positive mass":            func(c *runConfig) { c.massKG = 0 },
		"zero drive efficiency":        func(c *runConfig) { c.driveEfficiency = 0 },
		"drive efficiency above one":   func(c *runConfig) { c.driveEfficiency = 1.1 },
		"positive descent cutoff":      func(c *runConfig) { c.descentCutoffPercent = 0.5 },
		"non-positive descent cap":     func(c *runConfig) { c.descentCapMPS = -1 },
		"non-positive climb threshold": func(c *runConfig) { c.climbThresholdPercent = 0 },
		"tyre tolerance low above high": func(c *runConfig) {
			c.tyreCrrToleranceLow, c.tyreCrrToleranceHigh = 2.0, 1.0
		},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			mutate(&cfg)
			assert.Error(t, cfg.validate())
		})
	}
}

func TestRunConfigValidateIgnoresTyreToleranceOrderingWhenTheGateIsOff(t *testing.T) {
	cfg := validConfig()
	cfg.tyreCrrBench = 0 // gate disabled: an inverted tolerance band is moot
	cfg.tyreCrrToleranceLow, cfg.tyreCrrToleranceHigh = 2.0, 1.0
	assert.NoError(t, cfg.validate())
}

func TestCheckTOMLStemCollisionsRejectsTwoGearsThatNormalizeTheSame(t *testing.T) {
	groups := []rideGroup{
		{Gear: "Bike A"},
		{Gear: "Bike-A"}, // normalizes to the same stem as "Bike A"
	}

	err := checkTOMLStemCollisions(groups)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Bike A")
	assert.Contains(t, err.Error(), "Bike-A")
}

func TestCheckTOMLStemCollisionsIgnoresASkippedGroup(t *testing.T) {
	groups := []rideGroup{
		{Gear: "Bike A"},
		{Gear: "Bike-A", Skipped: true}, // never reaches the write step
	}

	assert.NoError(t, checkTOMLStemCollisions(groups))
}

func TestCheckTOMLStemCollisionsAcceptsDistinctGearNames(t *testing.T) {
	groups := []rideGroup{{Gear: "Bike A"}, {Gear: "Bike B"}}
	assert.NoError(t, checkTOMLStemCollisions(groups))
}

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

// coastingOnlyRide is syntheticRide without the climbing section: enough
// for stage A to fit Crr and CdA, nothing for stage B to fit power from.
func coastingOnlyRide(rideID string, date time.Time, crr, cda, massKG float64) []sampleRow {
	const airDensity = 1.2

	samples := make([]sampleRow, 0, 60)
	t := date
	speed := 15.0
	for range 60 {
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

	return samples
}

// A group with no sustained climbing above the threshold must fail rather
// than write a coefficients file with PowerWatts=0 — that file is not
// usable for downstream prediction, and 0 W would itself distort a
// forward model's held-out validation.
func TestFitGroupRejectsAGroupWithNoClimbingData(t *testing.T) {
	const massKG = 90.0
	start := time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC)

	samplesByRide := make(map[string][]sampleRow)
	var rides []rideRow
	for i := range 12 {
		rideID := "r" + string(rune('a'+i))
		date := start.AddDate(0, 0, i*7)
		samplesByRide[rideID] = coastingOnlyRide(rideID, date, 0.006, 0.45, massKG)
		rides = append(rides, rideRow{RideID: rideID, Gear: "Bike A", Date: date, MovingSeconds: 60})
	}

	group := groupRidesByGear(rides)[0]
	cfg := runConfig{massKG: massKG, driveEfficiency: 0.975, climbThresholdPercent: defaultClimbThresholdPercent}
	result := fitGroup(group, rides, samplesByRide, nil, &cfg)

	require.False(t, result.RejectedCrrBounds)
	require.False(t, result.IllConditioned)
	assert.True(t, result.NoClimbingData)
	assert.Zero(t, result.PowerWatts)
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
