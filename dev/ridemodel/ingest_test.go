package main

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pointAt(t time.Time, lat, lon, altitude, distance float64) point {
	return point{
		Time: t, HasPosition: true, Latitude: lat, Longitude: lon,
		HasAltitude: true, AltitudeMetres: altitude,
		HasDistance: true, DistanceMetres: distance,
	}
}

func TestBuildSamplesReflectsAGapInTheRecordStream(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []point{
		pointAt(start, 50, 8, 100, 0),
		pointAt(start.Add(time.Second), 50, 8, 100, 10),
		// A five-minute auto-pause gap, as a real device's own stream has.
		pointAt(start.Add(5*time.Minute+2*time.Second), 50, 8, 100, 20),
	}

	samples := buildSamples("r1", records, false)
	require.Len(t, samples, 2)
	assert.InDelta(t, 1.0, samples[0].DeltaSeconds, 0.001, "no gap here")
	assert.InDelta(t, 301.0, samples[1].DeltaSeconds, 0.001, "the gap must show up as this sample's own Δt")
}

func TestBuildSamplesSkipsAPairWithNoPositiveDelta(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []point{
		pointAt(start, 50, 8, 100, 0),
		pointAt(start, 50, 8, 100, 5), // duplicate timestamp
		pointAt(start.Add(time.Second), 50, 8, 100, 10),
	}

	samples := buildSamples("r1", records, false)
	require.Len(t, samples, 1, "the zero-Δt pair must be skipped, not divided by zero")
}

// Irregularly spaced records must still produce a Δt-weighted mean speed that
// matches the ride's own distance over moving time — a naive per-record
// average would over-weight the sparsely sampled stretch.
func TestBuildSamplesProducesADeltaWeightedMeanSpeedMatchingDistanceOverTime(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []point{
		pointAt(start, 50, 8, 100, 0),
		pointAt(start.Add(1*time.Second), 50, 8, 100, 10), // fast, 1s spacing
		pointAt(start.Add(2*time.Second), 50, 8, 100, 20),
		pointAt(start.Add(62*time.Second), 50, 8, 100, 220), // sparse, 60s spacing
	}

	samples := buildSamples("r1", records, false)
	require.Len(t, samples, 3)

	totalDelta, weightedSpeed := 0.0, 0.0
	for _, s := range samples {
		totalDelta += s.DeltaSeconds
		weightedSpeed += s.SpeedMetresPerSecond * s.DeltaSeconds
	}
	weightedMean := weightedSpeed / totalDelta

	totalDistance := records[len(records)-1].DistanceMetres - records[0].DistanceMetres
	totalTime := records[len(records)-1].Time.Sub(records[0].Time).Seconds()
	assert.InDelta(t, totalDistance/totalTime, weightedMean, 1e-9)

	// A naive per-record average would instead weight the one sparse, slow
	// 60s sample the same as each fast 1s one, and read noticeably higher.
	naiveMean := (samples[0].SpeedMetresPerSecond + samples[1].SpeedMetresPerSecond + samples[2].SpeedMetresPerSecond) / 3
	assert.Greater(t, naiveMean-weightedMean, 0.01)
}

func TestBuildSamplesMarksBelowThresholdAsNotMoving(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []point{
		pointAt(start, 50, 8, 100, 0),
		// 0.1 m over 1s: 0.1 m/s, under the threshold.
		pointAt(start.Add(time.Second), 50, 8, 100, 0.1),
		// 5 m over the next 1s: 5 m/s, briskly over it.
		pointAt(start.Add(2*time.Second), 50, 8, 100, 5.1),
	}

	samples := buildSamples("r1", records, false)
	require.Len(t, samples, 2)
	assert.False(t, samples[0].MovingFilter)
	assert.True(t, samples[1].MovingFilter)
}

// The window is a physical distance, so doubling how densely a ride is
// A point less than gradientWindowMetres into the ride has no full window
// behind it yet, and must report no gradient rather than one measured over
// whatever short span happens to be available — the same rule
// internal/route.Route.MaxGradientPercent applies.
func TestWindowedGradientsReportsNoneBeforeAFullWindowIsAvailable(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	// A steep, unmistakable grade, but only 40 m into the ride — well short
	// of the 100 m window.
	records := []point{
		pointAt(start, 50, 8, 0, 0),
		pointAt(start.Add(5*time.Second), 50, 8, 4, 20),
		pointAt(start.Add(10*time.Second), 50, 8, 8, 40),
	}
	gradients := windowedGradients(records, cumulativeDistances(records))
	assert.InDelta(t, 0, gradients[1], 1e-9)
	assert.InDelta(t, 0, gradients[2], 1e-9)
}

// sampled — inserting a point halfway between each original pair — must
// leave the gradient reported at each original point materially unchanged.
func TestWindowedGradientsAreInvariantToSampleDensity(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	// A steady 3% grade over 500 m, one point every 25 m / 5 s.
	sparse := make([]point, 0, 21)
	for i := 0; i <= 20; i++ {
		distance := float64(i) * 25
		sparse = append(sparse, pointAt(start.Add(time.Duration(i*5)*time.Second), 50, 8, distance*0.03, distance))
	}
	dense := make([]point, 0, 41)
	for i := 0; i <= 40; i++ {
		distance := float64(i) * 12.5
		dense = append(dense, pointAt(start.Add(time.Duration(float64(i)*2.5)*time.Second), 50, 8, distance*0.03, distance))
	}

	sparseCumulative := cumulativeDistances(sparse)
	denseCumulative := cumulativeDistances(dense)
	sparseGradients := windowedGradients(sparse, sparseCumulative)
	denseGradients := windowedGradients(dense, denseCumulative)

	// Compare the gradient at the same physical point (400 m in) in both.
	assert.InDelta(t, sparseGradients[16], denseGradients[32], 0.05)
	assert.InDelta(t, 3.0, sparseGradients[16], 0.05)
}

func TestSumRawRiseCountsOnlyPositiveSteps(t *testing.T) {
	records := []point{
		{HasAltitude: true, AltitudeMetres: 100},
		{HasAltitude: true, AltitudeMetres: 110}, // +10
		{HasAltitude: true, AltitudeMetres: 105}, // -5, not counted
		{HasAltitude: true, AltitudeMetres: 115}, // +10
		{HasAltitude: false},                     // skipped entirely
		{HasAltitude: true, AltitudeMetres: 120}, // +5 from the last altitude seen (115)
	}
	assert.InDelta(t, 25.0, sumRawRise(records), 0.001)
}

func TestIsCyclingTypeAcceptsRideVariantsAndRejectsOthers(t *testing.T) {
	for _, name := range []string{"Ride", "Virtual Ride", "E-Bike Ride", "Gravel Ride"} {
		assert.Truef(t, isCyclingType(name), "%q should be a ride", name)
	}
	for _, name := range []string{"Run", "Walk", "Rock Climb"} {
		assert.Falsef(t, isCyclingType(name), "%q should not be a ride", name)
	}
}

func TestIsIndoorTypeAcceptsBothSpellings(t *testing.T) {
	assert.True(t, isIndoorType("Virtual Ride"))
	assert.True(t, isIndoorType("VirtualRide"))
	assert.False(t, isIndoorType("Ride"))
}

// An indoor ride must be caught by carrying no position on any record, not
// only by its Strava activity type — a trainer session logged as plain
// "Ride" is exactly this case — while an outdoor ride with real GPS is
// untouched by the check.
func TestIngestActivityDetectsIndoorByPositionAsWellAsType(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	trainerRide := buildFITActivity(t, []testRecord{
		{at: start, noPosition: true, altitudeMetres: 0, distanceMetres: 0},
		{at: start.Add(time.Second), noPosition: true, altitudeMetres: 0, distanceMetres: 5},
	})
	exportDir := t.TempDir()
	require.NoError(t, os.MkdirAll(exportDir+"/activities", 0o750))
	writeGzippedFile(t, exportDir+"/activities", "trainer.fit.gz", encodeFIT(t, trainerRide))

	summary, samples, indoorRows := ingestActivity(exportDir, &activityRow{
		ID: "1", Type: "Ride", Filename: "activities/trainer.fit.gz",
	})
	assert.True(t, summary.Indoor)
	assert.Equal(t, exclusionIndoor, summary.Reason)
	assert.Nil(t, samples)
	assert.NotEmpty(t, indoorRows)
	assert.InDelta(t, 1.0, summary.ElapsedSeconds, 0.001, "an indoor ride's elapsed time must still be reported")

	outdoorRide := buildFITActivity(t, []testRecord{
		{at: start, lat: 50, lon: 8, altitudeMetres: 100, distanceMetres: 0},
		{at: start.Add(time.Second), lat: 50.0001, lon: 8, altitudeMetres: 101, distanceMetres: 10},
	})
	writeGzippedFile(t, exportDir+"/activities", "outdoor.fit.gz", encodeFIT(t, outdoorRide))
	summary2, samples2, _ := ingestActivity(exportDir, &activityRow{
		ID: "2", Type: "Ride", Filename: "activities/outdoor.fit.gz",
	})
	assert.False(t, summary2.Indoor)
	assert.False(t, summary2.Excluded)
	assert.NotEmpty(t, samples2)
}

func TestIngestActivityExcludesARideWithNoAltitudeUnderItsOwnReason(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	ride := buildFITActivity(t, []testRecord{
		{at: start, lat: 50, lon: 8, noAltitude: true, distanceMetres: 0},
		{at: start.Add(time.Second), lat: 50.0001, lon: 8, noAltitude: true, distanceMetres: 5},
	})
	exportDir := t.TempDir()
	require.NoError(t, os.MkdirAll(exportDir+"/activities", 0o750))
	writeGzippedFile(t, exportDir+"/activities", "no-alt.fit.gz", encodeFIT(t, ride))

	summary, samples, _ := ingestActivity(exportDir, &activityRow{
		ID: "1", Type: "Ride", Filename: "activities/no-alt.fit.gz",
	})
	assert.True(t, summary.Excluded)
	assert.Equal(t, exclusionNoAltitude, summary.Reason)
	assert.NotEqual(t, exclusionUnreadable, summary.Reason)
	assert.Nil(t, samples)
}

// A file that decodes but yields fewer than two usable record intervals must
// not count as ingested: buildSamples has nothing to emit for it, and a ride
// left non-excluded with zero rows in samples.csv would make the corpus's own
// ride count inconsistent with what it actually holds.
func TestIngestActivityExcludesARideWithFewerThanTwoRecords(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	ride := buildFITActivity(t, []testRecord{
		{at: start, lat: 50, lon: 8, altitudeMetres: 100, distanceMetres: 0},
	})
	exportDir := t.TempDir()
	require.NoError(t, os.MkdirAll(exportDir+"/activities", 0o750))
	writeGzippedFile(t, exportDir+"/activities", "one-record.fit.gz", encodeFIT(t, ride))

	summary, samples, indoor := ingestActivity(exportDir, &activityRow{
		ID: "1", Type: "Ride", Filename: "activities/one-record.fit.gz",
	})
	assert.True(t, summary.Excluded)
	assert.Equal(t, exclusionNoSamples, summary.Reason)
	assert.Nil(t, samples)
	assert.Nil(t, indoor)
}

func TestIngestActivityExcludesANonCyclingRow(t *testing.T) {
	summary, samples, indoor := ingestActivity(t.TempDir(), &activityRow{ID: "1", Type: "Run", Filename: "activities/x.fit.gz"})
	assert.True(t, summary.Excluded)
	assert.Equal(t, exclusionNotCycling, summary.Reason)
	assert.Nil(t, samples)
	assert.Nil(t, indoor)
}

// A Filename this tool did not write — an absolute path, or one built from
// ".." — must never be opened, whether the export is malformed or tampered
// with.
func TestIngestActivityRejectsAFilenameThatEscapesTheExportDirectory(t *testing.T) {
	for name, filename := range map[string]string{
		"absolute path":    "/etc/passwd",
		"parent traversal": "../../../etc/passwd",
		"embedded parent":  "activities/../../secret.fit.gz",
	} {
		t.Run(name, func(t *testing.T) {
			summary, samples, indoor := ingestActivity(t.TempDir(), &activityRow{
				ID: "1", Type: "Ride", Filename: filename,
			})
			assert.True(t, summary.Excluded)
			assert.Equal(t, exclusionUnsafeFilename, summary.Reason)
			assert.Nil(t, samples)
			assert.Nil(t, indoor)
		})
	}
}

func TestResolveActivityFileAcceptsAnOrdinaryRelativePath(t *testing.T) {
	path, err := resolveActivityFile("/export", "activities/123.fit.gz")
	require.NoError(t, err)
	assert.Equal(t, "/export/activities/123.fit.gz", path)
}

func TestIngestActivityReportsAltitudeQualityAgainstTheDevicesOwnAscent(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	ride := buildFITActivity(t, []testRecord{
		{at: start, lat: 50, lon: 8, altitudeMetres: 100, distanceMetres: 0},
		{at: start.Add(time.Second), lat: 50.0001, lon: 8, altitudeMetres: 110, distanceMetres: 10},
		{at: start.Add(2 * time.Second), lat: 50.0002, lon: 8, altitudeMetres: 120, distanceMetres: 20},
	})
	ride.Sessions[0].SetTotalAscent(15)
	exportDir := t.TempDir()
	require.NoError(t, os.MkdirAll(exportDir+"/activities", 0o750))
	writeGzippedFile(t, exportDir+"/activities", "climb.fit.gz", encodeFIT(t, ride))

	summary, _, _ := ingestActivity(exportDir, &activityRow{ID: "1", Type: "Ride", Filename: "activities/climb.fit.gz"})
	require.False(t, summary.Excluded)
	assert.InDelta(t, 20.0, summary.RawRiseMetres, 0.5) // 10 + 10 raw rise
	require.True(t, summary.HasAltitudeQuality)
	assert.InDelta(t, 15.0, summary.DeviceAscentMetres, 0.5)
}
