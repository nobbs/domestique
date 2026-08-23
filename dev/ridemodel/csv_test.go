package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A real Strava export repeats "Elapsed Time" and "Distance": once in an
// early summary block in mixed units, again in a later detailed block in
// metric base units (metres, seconds). This fixture reproduces that shape —
// the first Distance is kilometres, the second is metres — so a regression
// that goes back to "first occurrence wins" shows up as a thousand-fold
// distance error rather than a silent one.
const sampleActivitiesCSV = `Activity ID,Activity Date,Activity Type,Elapsed Time,Distance,Activity Gear,Filename,Elapsed Time,Moving Time,Distance,Elevation Gain
19865784256,"Aug 23, 2026, 10:36:58 AM",Ride,12691,78.95,Koga Colmaro Extreme,activities/21000736315.fit.gz,12691.0,11913.0,78957.9,406.0
17902963176,"Aug 20, 2026, 6:00:00 AM",Virtual Ride,3600,20.0,,activities/19002753344.fit.gz,3600.0,3550.0,20123.4,0.0
99999999999,"Aug 19, 2026, 7:00:00 AM",Rock Climb,1800,,,,,,,
`

func TestReadActivitiesCSVPrefersTheDetailedMetricColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activities.csv")
	require.NoError(t, os.WriteFile(path, []byte(sampleActivitiesCSV), 0o600))

	rows, err := readActivitiesCSV(path)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	ride := rows[0]
	assert.Equal(t, "19865784256", ride.ID)
	assert.Equal(t, "Ride", ride.Type)
	// The second, metres Distance column must win over the first, km one.
	assert.InDelta(t, 78957.9, ride.DistanceMetres, 0.1)
	assert.InDelta(t, 12691, ride.ElapsedTime.Seconds(), 0.01)
	assert.InDelta(t, 11913, ride.StravaMovingTime.Seconds(), 0.01)
	assert.InDelta(t, 406.0, ride.ElevationGainMetres, 0.01)
	assert.Equal(t, "Koga Colmaro Extreme", ride.Gear)
	assert.Equal(t, "activities/21000736315.fit.gz", ride.Filename)
	assert.Equal(t, 2026, ride.Date.Year())

	untagged := rows[1]
	assert.Empty(t, untagged.Gear, "an untagged ride's gear must be empty, not filled in")

	noFile := rows[2]
	assert.Empty(t, noFile.Filename)
}

func TestReadActivitiesCSVRejectsAMissingRequiredColumn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activities.csv")
	require.NoError(t, os.WriteFile(path, []byte("Activity Date,Activity Type\nAug 1, 2026, 6:00:00 AM,Ride\n"), 0o600))

	_, err := readActivitiesCSV(path)
	require.Error(t, err)
}
