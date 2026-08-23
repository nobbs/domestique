package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHRPowerRelationByYearRecoversAKnownLinearRelation(t *testing.T) {
	start := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)
	var rows []indoorRow
	for i := range 400 {
		hr := 120.0 + float64(i%60)
		power := 2.5*hr - 150 // known relation: slope 2.5, intercept -150
		rows = append(rows, indoorRow{
			Time: start.Add(time.Duration(i) * time.Second), HeartRateBPM: hr, HasHeartRate: true,
			PowerWatts: power, HasPower: true,
		})
	}

	relations := hrPowerRelationByYear(rows)
	require.Contains(t, relations, 2026)
	assert.InDelta(t, 2.5, relations[2026].Slope, 0.01)
	assert.InDelta(t, -150, relations[2026].Intercept, 1.0)
}

func TestHRPowerRelationByYearFlagsAThinYear(t *testing.T) {
	start := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)
	rows := []indoorRow{
		{Time: start, HeartRateBPM: 130, HasHeartRate: true, PowerWatts: 150, HasPower: true},
		{Time: start.Add(time.Second), HeartRateBPM: 140, HasHeartRate: true, PowerWatts: 170, HasPower: true},
	}

	relations := hrPowerRelationByYear(rows)
	require.Contains(t, relations, 2026)
	assert.True(t, relations[2026].Thin)
}

func TestCrossCheckRideNeedsBothChannelsToProduceAResult(t *testing.T) {
	samples := []sampleRow{
		{RideID: "r1", Time: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), DeltaSeconds: 1, HasHeartRate: false, HasAltitude: false},
	}
	relations := map[int]hrPowerRelation{2026: {Slope: 1, Intercept: 0}}

	_, ok := crossCheckRide(samples, relations, nil, 0.006, 0.45, 90, 0.975)
	assert.False(t, ok)
}

func TestCrossCheckRideAveragesBothPowerEstimatesOverTheRide(t *testing.T) {
	when := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	samples := []sampleRow{
		{
			RideID: "r1", Time: when, DeltaSeconds: 10, HasHeartRate: true, HeartRateBPM: 140,
			HasAltitude: true, Moving: true, SpeedMPS: 5.0, GradientPercent: 3.0,
		},
	}
	relations := map[int]hrPowerRelation{2026: {Slope: 1.5, Intercept: 0}}

	check, ok := crossCheckRide(samples, relations, nil, 0.006, 0.45, 90, 0.975)
	require.True(t, ok)
	assert.InDelta(t, 210.0, check.HRPower, 1e-9) // 1.5 * 140
	assert.Greater(t, check.PhysicsPower, 0.0)
}

func TestSummarizeCrossCheckComputesMedianRatioAndCorrelation(t *testing.T) {
	checks := []rideCrossCheck{
		{RideID: "r1", HRPower: 130, PhysicsPower: 130},
		{RideID: "r2", HRPower: 140, PhysicsPower: 140},
		{RideID: "r3", HRPower: 150, PhysicsPower: 150},
		{RideID: "r4", HRPower: 160, PhysicsPower: 160},
		{RideID: "r5", HRPower: 170, PhysicsPower: 187}, // 10% high, the outlier
	}

	summary := summarizeCrossCheck(checks, nil)
	assert.Equal(t, 5, summary.Rides)
	assert.InDelta(t, 1.0, summary.MedianRatio, 0.05)
	assert.Greater(t, summary.Correlation, 0.9)
}
