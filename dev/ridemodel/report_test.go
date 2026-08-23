package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBuildReportGroupsGearIncludingUntagged(t *testing.T) {
	rides := []rideSummary{
		{RideID: "1", Gear: "Bike A", ElapsedSeconds: 3600},
		{RideID: "2", Gear: "Bike A", ElapsedSeconds: 3600},
		{RideID: "3", Gear: "", ElapsedSeconds: 1800},
	}
	data := buildReport(rides)
	assert.Equal(t, 3, data.ingested)
	assert.Equal(t, gearStat{rides: 2, hours: 2}, data.gear["Bike A"])
	assert.Equal(t, gearStat{rides: 1, hours: 0.5}, data.gear[""])
}

func TestBuildReportSeparatesIndoorAndExcludedFromIngested(t *testing.T) {
	rides := []rideSummary{
		{RideID: "1", ElapsedSeconds: 3600},
		{RideID: "2", Indoor: true, Excluded: true, Reason: exclusionIndoor, ElapsedSeconds: 1800},
		{RideID: "3", Excluded: true, Reason: exclusionNoAltitude},
		{RideID: "4", Excluded: true, Reason: exclusionNotCycling},
	}
	data := buildReport(rides)
	assert.Equal(t, 1, data.ingested)
	assert.Equal(t, 1, data.indoorRides)
	assert.Equal(t, 1, data.excludedByReason[exclusionNoAltitude])
	assert.Equal(t, 1, data.excludedByReason[exclusionNotCycling])
}

func TestBuildReportGroupsCadenceByRecordingDeviceIncludingUnknown(t *testing.T) {
	rides := []rideSummary{
		{RideID: "1", RecordingDevice: "Garmin", HasCadence: true},
		{RideID: "2", RecordingDevice: "Garmin", HasCadence: false},
		{RideID: "3", RecordingDevice: "", HasCadence: false},
	}
	data := buildReport(rides)
	assert.Equal(t, deviceCadenceStat{rides: 2, withCadence: 1}, data.cadenceByDevice["Garmin"])
	assert.Equal(t, deviceCadenceStat{rides: 1, withCadence: 0}, data.cadenceByDevice["unknown"])
}

func TestBuildReportFlagsATimerDivergenceBeyondTheThreshold(t *testing.T) {
	rides := []rideSummary{
		{RideID: "close", ElapsedSeconds: 3600, HasDeviceTimerTime: true, DeviceTimerTime: 3600 * time.Second * 98 / 100},
		{RideID: "far", ElapsedSeconds: 3600, HasDeviceTimerTime: true, DeviceTimerTime: 3600 * time.Second * 80 / 100},
	}
	data := buildReport(rides)
	assert.Equal(t, []string{"far"}, data.timerDivergentRides)
}

func TestPercentileMatchesKnownValues(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}
	assert.InDelta(t, 1, percentile(sorted, 0), 1e-9)
	assert.InDelta(t, 3, percentile(sorted, 0.5), 1e-9)
	assert.InDelta(t, 5, percentile(sorted, 1), 1e-9)
	assert.InDelta(t, 2, percentile(sorted, 0.25), 1e-9)
}

func TestRenderReportDoesNotPanicOnAnEmptyRun(t *testing.T) {
	data := buildReport(nil)
	assert.NotPanics(t, func() { renderReport(&data) })
}
