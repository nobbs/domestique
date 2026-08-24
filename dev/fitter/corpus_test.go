package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeCSV(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestReadSamplesCSVReadsAnOrdinaryRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.csv")
	writeCSV(t, path, "ride_id,time,delta_seconds,interval_distance_m,speed_mps,gradient_percent,"+
		"altitude_m,has_altitude,cadence_rpm,has_cadence,latitude,longitude,has_position,moving,heart_rate_bpm,has_heart_rate\n"+
		"r1,2026-08-01T06:00:00Z,1,5,5,2.5,100,true,80,true,50.0,8.0,true,true,140,true\n")

	rows, err := readSamplesCSV(path)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "r1", rows[0].RideID)
	assert.InDelta(t, 5.0, rows[0].IntervalDistance, 1e-9)
	assert.InDelta(t, 80.0, rows[0].CadenceRPM, 1e-9)
	assert.True(t, rows[0].HasCadence)
	assert.InDelta(t, 50.0, rows[0].Latitude, 1e-9)
	assert.True(t, rows[0].HasPosition)
	assert.True(t, rows[0].Moving)
	assert.InDelta(t, 140.0, rows[0].HeartRateBPM, 1e-9)
	assert.True(t, rows[0].HasHeartRate)
}

// samples.csv is a contract with dev/ridemodel, not arbitrary input: a
// missing required column means the file is not the corpus this package
// expects, and reading zero values for it silently would let a fit run
// against empty data with nothing to say why.
func TestReadSamplesCSVFailsOnAMissingRequiredColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.csv")
	writeCSV(t, path, "time,delta_seconds,interval_distance_m,speed_mps,gradient_percent,"+
		"altitude_m,has_altitude,cadence_rpm,has_cadence,latitude,longitude,has_position,moving\n"+
		"2026-08-01T06:00:00Z,1,5,5,2.5,100,true,80,true,50.0,8.0,true,true\n") // no ride_id column

	_, err := readSamplesCSV(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ride_id")
}

func TestReadRidesCSVReadsAnOrdinaryRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rides.csv")
	writeCSV(t, path, "ride_id,date,gear,moving_seconds\n"+
		"r1,2026-08-01T06:00:00Z,Bike A,3600\n")

	rows, err := readRidesCSV(path)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Bike A", rows[0].Gear)
	assert.InDelta(t, 3600.0, rows[0].MovingSeconds, 1e-9)
}

func TestReadRidesCSVFailsOnAMissingRequiredColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rides.csv")
	writeCSV(t, path, "ride_id,gear,moving_seconds\nr1,Bike A,3600\n") // no date column

	_, err := readRidesCSV(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "date")
}

func TestReadIndoorCSVReadsAnOrdinaryRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indoor.csv")
	writeCSV(t, path, "ride_id,time,delta_seconds,power_w,has_power,heart_rate_bpm,has_heart_rate\n"+
		"r1,2026-08-01T06:00:00Z,1,180,true,150,true\n")

	rows, err := readIndoorCSV(path)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.InDelta(t, 180.0, rows[0].PowerWatts, 1e-9)
	assert.True(t, rows[0].HasPower)
}

func TestReadIndoorCSVFailsOnAMissingRequiredColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "indoor.csv")
	writeCSV(t, path, "ride_id,power_w,has_power\nr1,180,true\n") // no time column

	_, err := readIndoorCSV(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "time")
}
