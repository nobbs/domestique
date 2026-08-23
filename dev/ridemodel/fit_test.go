package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
	"time"

	fitencoder "github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRecord is one record this test's synthetic rides are built from.
type testRecord struct {
	at                     time.Time
	lat, lon               float64
	altitudeMetres         float64
	distanceMetres         float64
	cadence                uint8
	temperature            int8
	noPosition, noAltitude bool
}

// buildFITActivity assembles a minimal but decodable Activity file: a FileId,
// one Session and the given records, mirroring the shape a real head unit
// export carries.
func buildFITActivity(t *testing.T, records []testRecord) *filedef.Activity {
	t.Helper()
	activity := &filedef.Activity{}
	activity.FileId.SetType(typedef.FileActivity).SetManufacturer(typedef.ManufacturerDevelopment).SetTimeCreated(records[0].at)

	first, last := records[0].at, records[len(records)-1].at
	session := mesgdef.NewSession(nil).
		SetTimestamp(last).
		SetStartTime(first).
		SetSport(typedef.SportCycling).
		SetTotalElapsedTimeScaled(last.Sub(first).Seconds()).
		SetTotalTimerTimeScaled(last.Sub(first).Seconds()).
		SetTotalAscent(0)
	activity.Sessions = append(activity.Sessions, session)
	activity.Activity = mesgdef.NewActivity(nil).SetTimestamp(last).SetNumSessions(1)

	for _, r := range records {
		record := mesgdef.NewRecord(nil).SetTimestamp(r.at)
		if !r.noPosition {
			record.SetPositionLatDegrees(r.lat).SetPositionLongDegrees(r.lon)
		}
		if !r.noAltitude {
			record.SetAltitudeScaled(r.altitudeMetres)
		}
		record.SetDistanceScaled(r.distanceMetres)
		if r.cadence != 0 {
			record.SetCadence(r.cadence)
		}
		if r.temperature != 0 {
			record.SetTemperature(r.temperature)
		}
		activity.Records = append(activity.Records, record)
	}

	return activity
}

func encodeFIT(t *testing.T, activity *filedef.Activity) []byte {
	t.Helper()
	fit := activity.ToFIT(nil)
	var buffer bytes.Buffer
	require.NoError(t, fitencoder.New(&buffer).Encode(&fit))

	return buffer.Bytes()
}

// writeGzippedFile gzips content and writes it under dir, returning the path.
func writeGzippedFile(t *testing.T, dir, name string, content []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	file, err := os.Create(path) //nolint:gosec // A fixture path under the test's own temp directory.
	require.NoError(t, err)
	defer closeFile(file)
	writer := gzip.NewWriter(file)
	_, err = writer.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	return path
}

func TestDecodeFITGZRoundTripsASyntheticRide(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []testRecord{
		{at: start, lat: 50.0, lon: 8.0, altitudeMetres: 100, distanceMetres: 0, cadence: 80},
		{at: start.Add(time.Second), lat: 50.0001, lon: 8.0, altitudeMetres: 101, distanceMetres: 11, cadence: 82},
		{at: start.Add(2 * time.Second), lat: 50.0002, lon: 8.0, altitudeMetres: 102, distanceMetres: 22, cadence: 81},
	}
	activity := buildFITActivity(t, records)
	path := writeGzippedFile(t, t.TempDir(), "ride.fit.gz", encodeFIT(t, activity))

	decoded, err := decodeFITGZ(path)
	require.NoError(t, err)
	require.False(t, decoded.ChecksumFailed)
	require.Len(t, decoded.Records, 3)
	assert.True(t, decoded.Records[0].Time.Equal(start))
	assert.True(t, decoded.Records[0].HasPosition)
	assert.InDelta(t, 50.0, decoded.Records[0].Latitude, 0.0001)
	assert.True(t, decoded.Records[0].HasAltitude)
	assert.InDelta(t, 100, decoded.Records[0].AltitudeMetres, 0.5)
	assert.True(t, decoded.Records[0].HasDistance)
	assert.InDelta(t, 22, decoded.Records[2].DistanceMetres, 0.5)
	assert.True(t, decoded.Records[0].HasCadence)
	assert.InDelta(t, 80, decoded.Records[0].CadenceRPM, 0)
	assert.True(t, decoded.HasTotalElapsedTime)
	assert.InDelta(t, 2, decoded.TotalElapsedTime.Seconds(), 0.01)
}

func TestDecodeFITGZDecodesAChecksumFailureAndCountsIt(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []testRecord{
		{at: start, lat: 50.0, lon: 8.0, altitudeMetres: 100, distanceMetres: 0},
		{at: start.Add(time.Second), lat: 50.0001, lon: 8.0, altitudeMetres: 101, distanceMetres: 11},
	}
	encoded := encodeFIT(t, buildFITActivity(t, records))
	// The file's final two bytes are its whole-file CRC. Flipping the last
	// byte corrupts it without touching anything the decoder parses before
	// reaching that check, the way Zwift's own broken files do.
	require.NotEmpty(t, encoded)
	encoded[len(encoded)-1] ^= 0xFF
	path := writeGzippedFile(t, t.TempDir(), "zwift.fit.gz", encoded)

	decoded, err := decodeFITGZ(path)
	require.NoError(t, err)
	assert.True(t, decoded.ChecksumFailed)
	assert.Len(t, decoded.Records, 2)
}

func TestDecodeFITGZRejectsAGenuinelyUnreadableFile(t *testing.T) {
	path := writeGzippedFile(t, t.TempDir(), "garbage.fit.gz", []byte("this is not a FIT file"))

	_, err := decodeFITGZ(path)
	require.Error(t, err)
}

func TestDecodeFITGZDetectsARideWithNoPosition(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []testRecord{
		{at: start, noPosition: true, altitudeMetres: 0, distanceMetres: 0},
		{at: start.Add(time.Second), noPosition: true, altitudeMetres: 0, distanceMetres: 5},
	}
	path := writeGzippedFile(t, t.TempDir(), "trainer.fit.gz", encodeFIT(t, buildFITActivity(t, records)))

	decoded, err := decodeFITGZ(path)
	require.NoError(t, err)
	for _, r := range decoded.Records {
		assert.False(t, r.HasPosition)
	}
}

func TestDecodeFITGZDetectsARideWithNoAltitude(t *testing.T) {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	records := []testRecord{
		{at: start, lat: 50, lon: 8, noAltitude: true, distanceMetres: 0},
		{at: start.Add(time.Second), lat: 50.0001, lon: 8, noAltitude: true, distanceMetres: 5},
	}
	path := writeGzippedFile(t, t.TempDir(), "no-altitude.fit.gz", encodeFIT(t, buildFITActivity(t, records)))

	decoded, err := decodeFITGZ(path)
	require.NoError(t, err)
	for _, r := range decoded.Records {
		assert.False(t, r.HasAltitude)
	}
}
