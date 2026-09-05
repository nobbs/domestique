package activity

import (
	"bytes"
	"testing"
	"time"

	fitencoder "github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeFITReadsActivityRecordsAndSession(t *testing.T) {
	start := time.Date(2026, time.August, 1, 6, 0, 0, 0, time.UTC)
	activity := &filedef.Activity{}
	activity.FileId.SetType(typedef.FileActivity).SetManufacturer(typedef.ManufacturerDevelopment)
	activity.Sessions = append(activity.Sessions, mesgdef.NewSession(nil).
		SetTotalTimerTimeScaled(120).
		SetTotalElapsedTimeScaled(125).
		SetTotalAscent(42))
	activity.Records = append(activity.Records, mesgdef.NewRecord(nil).
		SetTimestamp(start).
		SetPositionLatDegrees(50.0).
		SetPositionLongDegrees(8.0).
		SetEnhancedAltitudeScaled(100.4).
		SetDistanceScaled(12.5).
		SetCadence(84).
		SetTemperature(16).
		SetPower(250).
		SetHeartRate(150))

	decoded, err := DecodeFIT(encode(t, activity))
	require.NoError(t, err)
	require.Len(t, decoded.Records, 1)
	assert.False(t, decoded.ChecksumFailed)
	assert.Equal(t, typedef.ManufacturerDevelopment.String(), decoded.RecordingDevice)
	assert.True(t, decoded.HasTotalTimerTime)
	assert.Equal(t, 120*time.Second, decoded.TotalTimerTime)
	assert.True(t, decoded.HasTotalElapsedTime)
	assert.Equal(t, 125*time.Second, decoded.TotalElapsedTime)
	assert.True(t, decoded.HasTotalAscent)
	assert.InDelta(t, 42.0, decoded.TotalAscentMetres, 0)
	assert.Equal(t, start, decoded.Records[0].Time)
	assert.True(t, decoded.Records[0].HasPosition)
	assert.InDelta(t, 50.0, decoded.Records[0].Latitude, 0.0001)
	assert.InDelta(t, 8.0, decoded.Records[0].Longitude, 0.0001)
	assert.True(t, decoded.Records[0].HasAltitude)
	assert.InDelta(t, 100.4, decoded.Records[0].AltitudeMetres, 0.05, "enhanced altitude wins over plain")
	assert.True(t, decoded.Records[0].HasDistance)
	assert.InDelta(t, 12.5, decoded.Records[0].DistanceMetres, 0.1)
	assert.True(t, decoded.Records[0].HasCadence)
	assert.InDelta(t, 84.0, decoded.Records[0].CadenceRPM, 0)
	assert.True(t, decoded.Records[0].HasTemperatureCelsius)
	assert.InDelta(t, 16.0, decoded.Records[0].TemperatureCelsius, 0)
	assert.True(t, decoded.Records[0].HasPower)
	assert.InDelta(t, 250.0, decoded.Records[0].PowerWatts, 0)
	assert.True(t, decoded.Records[0].HasHeartRate)
	assert.InDelta(t, 150.0, decoded.Records[0].HeartRateBPM, 0)

	preferredAltitude := fromRecord(mesgdef.NewRecord(nil).SetAltitudeScaled(100).SetEnhancedAltitudeScaled(100.4))
	assert.InDelta(t, 100.4, preferredAltitude.AltitudeMetres, 0.1)
	plainAltitude := fromRecord(mesgdef.NewRecord(nil).SetAltitudeScaled(100))
	assert.InDelta(t, 100, plainAltitude.AltitudeMetres, 0.1)
	assert.False(t, fromRecord(mesgdef.NewRecord(nil)).HasAltitude)
}

func TestDecodeFITRecoversAReadableChecksumFailure(t *testing.T) {
	activity := &filedef.Activity{}
	activity.FileId.SetType(typedef.FileActivity)
	encoded := encode(t, activity)
	encoded[len(encoded)-1] ^= 0xFF

	decoded, err := DecodeFIT(encoded)
	require.NoError(t, err)
	assert.True(t, decoded.ChecksumFailed)
}

func TestDecodeFITRejectsUnreadableAndNonActivityFiles(t *testing.T) {
	_, err := DecodeFIT([]byte("not a FIT file"))
	require.Error(t, err)

	course := encode(t, filedef.NewCourse())
	_, err = DecodeFIT(course)
	require.ErrorContains(t, err, "no activity file type found")

	course[len(course)-1] ^= 0xFF
	_, err = DecodeFIT(course)
	require.ErrorContains(t, err, "with checksum ignored")
}

func encode(t *testing.T, file filedef.File) []byte {
	t.Helper()
	fit := file.ToFIT(nil)
	var buffer bytes.Buffer
	require.NoError(t, fitencoder.New(&buffer).Encode(&fit))

	return buffer.Bytes()
}
