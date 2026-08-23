package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleGPX = `<?xml version="1.0" encoding="UTF-8"?>
<gpx creator="StravaGPX" version="1.1" xmlns="http://www.topografix.com/GPX/1/1">
 <trk>
  <name>Morning Ride</name>
  <trkseg>
   <trkpt lat="50.00298" lon="8.26014">
    <ele>93.8</ele>
    <time>2026-08-01T06:00:00Z</time>
   </trkpt>
   <trkpt lat="50.00300" lon="8.26020">
    <ele>94.1</ele>
    <time>2026-08-01T06:00:10Z</time>
    <extensions>
     <gpxtpx:TrackPointExtension>
      <gpxtpx:cad>82</gpxtpx:cad>
      <gpxtpx:hr>128</gpxtpx:hr>
     </gpxtpx:TrackPointExtension>
    </extensions>
   </trkpt>
  </trkseg>
 </trk>
</gpx>
`

func TestDecodeGPXReadsAPlainFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ride.gpx")
	require.NoError(t, os.WriteFile(path, []byte(sampleGPX), 0o600))

	decoded, err := decodeGPX(path, false)
	require.NoError(t, err)
	assert.True(t, decoded.Derived)
	require.Len(t, decoded.Records, 2)
	assert.True(t, decoded.Records[0].HasPosition)
	assert.InDelta(t, 50.00298, decoded.Records[0].Latitude, 0.00001)
	assert.True(t, decoded.Records[0].HasAltitude)
	assert.InDelta(t, 93.8, decoded.Records[0].AltitudeMetres, 0.01)
	assert.False(t, decoded.Records[0].HasCadence)
	assert.True(t, decoded.Records[1].HasCadence)
	assert.InDelta(t, 82, decoded.Records[1].CadenceRPM, 0)
	assert.True(t, decoded.Records[1].HasHeartRate)
}

func TestDecodeGPXReadsAGzippedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ride.gpx.gz")
	file, err := os.Create(path) //nolint:gosec // A fixture path under the test's own temp directory.
	require.NoError(t, err)
	writer := gzip.NewWriter(file)
	_, err = writer.Write([]byte(sampleGPX))
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	require.NoError(t, file.Close())

	decoded, err := decodeGPX(path, true)
	require.NoError(t, err)
	require.Len(t, decoded.Records, 2)
}

func TestDecodeGPXDropsAPointWithAnUnparseableTime(t *testing.T) {
	broken := bytes.Replace([]byte(sampleGPX), []byte("2026-08-01T06:00:00Z"), []byte("not-a-time"), 1)
	path := filepath.Join(t.TempDir(), "ride.gpx")
	require.NoError(t, os.WriteFile(path, broken, 0o600))

	decoded, err := decodeGPX(path, false)
	require.NoError(t, err)
	require.Len(t, decoded.Records, 1)
}
