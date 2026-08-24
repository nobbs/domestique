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
			PowerWatts: power, HasPower: true, DeltaSeconds: 1,
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
		{Time: start, HeartRateBPM: 130, HasHeartRate: true, PowerWatts: 150, HasPower: true, DeltaSeconds: 1},
		{Time: start.Add(time.Second), HeartRateBPM: 140, HasHeartRate: true, PowerWatts: 170, HasPower: true, DeltaSeconds: 1},
	}

	relations := hrPowerRelationByYear(rows)
	require.Contains(t, relations, 2026)
	assert.True(t, relations[2026].Thin)
}

// A single long-interval row must count for as much as the seconds it
// covers, not as one vote among many rows: indoor.csv is not strictly
// 1 Hz, and #214's own Δt-weighting invariant is what this test checks
// survived the port. Proven by an invariance a per-row OLS would not have:
// one 50 s row at a given (HR, power) must move the fit exactly as much as
// fifty separate 1 s rows at that same (HR, power) would.
func TestHRPowerRelationByYearWeightsByDeltaSecondsNotRowCount(t *testing.T) {
	start := time.Date(2026, 3, 1, 6, 0, 0, 0, time.UTC)
	background := func() []indoorRow {
		var rows []indoorRow
		for i := range 60 {
			hr := 120.0 + float64(i%40)
			rows = append(rows, indoorRow{
				Time: start.Add(time.Duration(i) * time.Second), HeartRateBPM: hr, HasHeartRate: true,
				PowerWatts: 2.0*hr - 100, HasPower: true, DeltaSeconds: 1,
			})
		}

		return rows
	}

	oneLongRow := append(background(), indoorRow{
		Time: start.Add(time.Hour), HeartRateBPM: 200, HasHeartRate: true, PowerWatts: 500, HasPower: true, DeltaSeconds: 50,
	})
	manyShortRows := background()
	for i := range 50 {
		manyShortRows = append(manyShortRows, indoorRow{
			Time:         start.Add(time.Hour).Add(time.Duration(i) * time.Second),
			HeartRateBPM: 200, HasHeartRate: true, PowerWatts: 500, HasPower: true, DeltaSeconds: 1,
		})
	}

	fromOneLongRow := hrPowerRelationByYear(oneLongRow)[2026]
	fromManyShortRows := hrPowerRelationByYear(manyShortRows)[2026]
	assert.InDelta(t, fromManyShortRows.Slope, fromOneLongRow.Slope, 1e-9)
	assert.InDelta(t, fromManyShortRows.Intercept, fromOneLongRow.Intercept, 1e-6)
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

// A ride with no usable HRPower must not count toward Rides or feed the
// correlation while being excluded from MedianRatio — all three figures
// describe the same subset, or they are not comparable.
func TestSummarizeCrossCheckExcludesARideWithNoUsableHRPowerFromEveryFigure(t *testing.T) {
	checks := []rideCrossCheck{
		{RideID: "r1", HRPower: 130, PhysicsPower: 130},
		{RideID: "r2", HRPower: 140, PhysicsPower: 140},
		{RideID: "r3", HRPower: 0, PhysicsPower: 9999}, // no usable HRPower, would skew correlation
	}

	summary := summarizeCrossCheck(checks, nil)
	assert.Equal(t, 2, summary.Rides)
	assert.InDelta(t, 1.0, summary.MedianRatio, 1e-9)
}

func TestSummarizeCrossCheckReturnsZeroValueWhenNoRideHasAUsableRatio(t *testing.T) {
	checks := []rideCrossCheck{{RideID: "r1", HRPower: 0, PhysicsPower: 150}}
	summary := summarizeCrossCheck(checks, nil)
	assert.Equal(t, indoorCrossCheckSummary{}, summary)
}

func TestSummarizeCrossCheckSortsThinYearsForDeterministicOutput(t *testing.T) {
	checks := []rideCrossCheck{{RideID: "r1", HRPower: 100, PhysicsPower: 100}}
	relations := map[int]hrPowerRelation{2026: {Thin: true}, 2024: {Thin: true}, 2025: {Thin: true}}

	summary := summarizeCrossCheck(checks, relations)
	assert.Equal(t, []int{2024, 2025, 2026}, summary.ThinYears)
}
