package activity

import (
	"bytes"
	"testing"
	"time"

	fitencoder "github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
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
	assert.True(t, decoded.Session.TimerSeconds.Known)
	assert.InDelta(t, 120.0, decoded.Session.TimerSeconds.Value, 0)
	assert.True(t, decoded.Session.ElapsedSeconds.Known)
	assert.InDelta(t, 125.0, decoded.Session.ElapsedSeconds.Value, 0)
	assert.True(t, decoded.Session.AscentMetres.Known)
	assert.InDelta(t, 42.0, decoded.Session.AscentMetres.Value, 0)
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

	preferredAltitude := fromRecord(mesgdef.NewRecord(nil).SetAltitudeScaled(100).SetEnhancedAltitudeScaled(100.4), nil)
	assert.InDelta(t, 100.4, preferredAltitude.AltitudeMetres, 0.1)
	plainAltitude := fromRecord(mesgdef.NewRecord(nil).SetAltitudeScaled(100), nil)
	assert.InDelta(t, 100, plainAltitude.AltitudeMetres, 0.1)
	assert.False(t, fromRecord(mesgdef.NewRecord(nil), nil).HasAltitude)
}

// TestDecodeFITReadsSessionAndZoneMessages exercises every session field this
// package decodes, plus hr_zone and power_zone messages arriving out of
// message_index order.
func TestDecodeFITReadsSessionAndZoneMessages(t *testing.T) {
	activityFile := &filedef.Activity{}
	activityFile.FileId.SetType(typedef.FileActivity)
	activityFile.Sessions = append(activityFile.Sessions, mesgdef.NewSession(nil).
		SetEnhancedMaxSpeedScaled(17.686).
		SetEnhancedAvgSpeedScaled(8.5).
		SetTotalDistanceScaled(45000).
		SetTotalTimerTimeScaled(7200).
		SetTotalElapsedTimeScaled(7500).
		SetTotalAscent(650).
		SetTotalDescent(640).
		SetTotalCalories(1800).
		SetAvgHeartRate(140).
		SetMaxHeartRate(175).
		SetMinHeartRate(90).
		SetAvgCadence(85).
		SetMaxCadence(110).
		SetAvgPower(180).
		SetMaxPower(650).
		SetNormalizedPower(200).
		SetThresholdPower(265).
		SetAvgTemperature(18).
		SetMaxTemperature(24).
		SetAvgGradeScaled(1.5).
		SetMaxPosGradeScaled(12.5).
		SetMaxNegGradeScaled(-15.5).
		SetEnhancedMinAltitudeScaled(100.0).
		SetEnhancedMaxAltitudeScaled(800.5).
		SetEnhancedAvgAltitudeScaled(300.2).
		SetSport(typedef.SportCycling).
		SetSubSport(typedef.SubSportRoad).
		SetTimeInHrZoneScaled([]float64{100, 200, 300, 400, 500}))
	for i, high := range map[typedef.MessageIndex]uint8{2: 130, 0: 100, 1: 115, 4: 170, 3: 150} {
		activityFile.UnrelatedMessages = append(activityFile.UnrelatedMessages,
			mesgdef.NewHrZone(nil).SetMessageIndex(i).SetHighBpm(high).ToMesg(nil))
	}
	for i, high := range map[typedef.MessageIndex]uint16{1: 200, 0: 100} {
		activityFile.UnrelatedMessages = append(activityFile.UnrelatedMessages,
			mesgdef.NewPowerZone(nil).SetMessageIndex(i).SetHighValue(high).ToMesg(nil))
	}

	decoded, err := DecodeFIT(encode(t, activityFile))
	require.NoError(t, err)
	session := decoded.Session

	assert.InDelta(t, 63.6696, session.MaxSpeedKmh.Value, 0.001)
	assert.InDelta(t, 30.6, session.AverageSpeedKmh.Value, 0.001)
	assert.InDelta(t, 45000, session.DistanceMetres.Value, 0.1)
	assert.InDelta(t, 7200, session.TimerSeconds.Value, 0.1)
	assert.InDelta(t, 7500, session.ElapsedSeconds.Value, 0.1)
	assert.InDelta(t, 650, session.AscentMetres.Value, 0)
	assert.InDelta(t, 640, session.DescentMetres.Value, 0)
	assert.InDelta(t, 1800, session.CaloriesKcal.Value, 0)
	assert.InDelta(t, 140, session.AverageHeartRateBPM.Value, 0)
	assert.InDelta(t, 175, session.MaxHeartRateBPM.Value, 0)
	assert.InDelta(t, 90, session.MinHeartRateBPM.Value, 0)
	assert.InDelta(t, 85, session.AverageCadenceRPM.Value, 0)
	assert.InDelta(t, 110, session.MaxCadenceRPM.Value, 0)
	assert.InDelta(t, 180, session.AveragePowerWatts.Value, 0)
	assert.InDelta(t, 650, session.MaxPowerWatts.Value, 0)
	assert.InDelta(t, 200, session.NormalizedPowerWatts.Value, 0)
	assert.InDelta(t, 265, session.ThresholdPowerWatts.Value, 0)
	assert.InDelta(t, 18, session.AverageTemperatureCelsius.Value, 0)
	assert.InDelta(t, 24, session.MaxTemperatureCelsius.Value, 0)
	assert.InDelta(t, 1.5, session.AverageGradePercent.Value, 0.01)
	assert.InDelta(t, 12.5, session.MaxPositiveGradePercent.Value, 0.01)
	assert.InDelta(t, -15.5, session.MaxNegativeGradePercent.Value, 0.01)
	assert.InDelta(t, 100.0, session.MinAltitudeMetres.Value, 0.1)
	assert.InDelta(t, 800.5, session.MaxAltitudeMetres.Value, 0.1)
	assert.InDelta(t, 300.2, session.AverageAltitudeMetres.Value, 0.1)
	assert.Equal(t, typedef.SportCycling.String(), session.Sport)
	assert.Equal(t, typedef.SubSportRoad.String(), session.SubSport)
	assert.InDeltaSlice(t, []float64{100, 200, 300, 400, 500}, session.HeartRateZoneSeconds, 0.001)
	assert.Equal(t, []float64{100, 115, 130, 150, 170}, session.HeartRateZoneHighBPM,
		"hr_zone messages must come back sorted by message_index, not arrival order")
	assert.Equal(t, []float64{100, 200}, session.PowerZoneHighWatts,
		"power_zone messages must come back sorted by message_index, not arrival order")
	assert.True(t, session.Any())
}

// TestDecodeFITFallsBackToLegacySpeedAndAltitudeFields covers a session that
// only carries the pre-enhanced 16-bit fields.
func TestDecodeFITFallsBackToLegacySpeedAndAltitudeFields(t *testing.T) {
	activityFile := &filedef.Activity{}
	activityFile.FileId.SetType(typedef.FileActivity)
	activityFile.Sessions = append(activityFile.Sessions, mesgdef.NewSession(nil).
		SetMaxSpeedScaled(9.5).
		SetMaxAltitudeScaled(555.5))

	decoded, err := DecodeFIT(encode(t, activityFile))
	require.NoError(t, err)
	assert.True(t, decoded.Session.MaxSpeedKmh.Known)
	assert.InDelta(t, 34.2, decoded.Session.MaxSpeedKmh.Value, 0.01)
	assert.True(t, decoded.Session.MaxAltitudeMetres.Known)
	assert.InDelta(t, 555.5, decoded.Session.MaxAltitudeMetres.Value, 0.1)
}

// TestDecodeFITLeavesSessionUnknownWhenUnset asserts a session message that
// sets none of these fields leaves every Reading unknown, Sport empty, the
// zone slices nil, and Session.Any false.
func TestDecodeFITLeavesSessionUnknownWhenUnset(t *testing.T) {
	activityFile := &filedef.Activity{}
	activityFile.FileId.SetType(typedef.FileActivity)
	activityFile.Sessions = append(activityFile.Sessions, mesgdef.NewSession(nil).
		SetTimestamp(time.Date(2026, time.August, 1, 6, 0, 0, 0, time.UTC)))

	decoded, err := DecodeFIT(encode(t, activityFile))
	require.NoError(t, err)
	session := decoded.Session
	assert.False(t, session.MaxSpeedKmh.Known)
	assert.False(t, session.AverageSpeedKmh.Known)
	assert.False(t, session.DistanceMetres.Known)
	assert.False(t, session.TimerSeconds.Known)
	assert.False(t, session.ElapsedSeconds.Known)
	assert.False(t, session.AscentMetres.Known)
	assert.False(t, session.DescentMetres.Known)
	assert.False(t, session.CaloriesKcal.Known)
	assert.False(t, session.AverageHeartRateBPM.Known)
	assert.False(t, session.MaxHeartRateBPM.Known)
	assert.False(t, session.MinHeartRateBPM.Known)
	assert.False(t, session.AverageCadenceRPM.Known)
	assert.False(t, session.MaxCadenceRPM.Known)
	assert.False(t, session.AveragePowerWatts.Known)
	assert.False(t, session.MaxPowerWatts.Known)
	assert.False(t, session.NormalizedPowerWatts.Known)
	assert.False(t, session.ThresholdPowerWatts.Known)
	assert.False(t, session.AverageTemperatureCelsius.Known)
	assert.False(t, session.MaxTemperatureCelsius.Known)
	assert.False(t, session.AverageGradePercent.Known)
	assert.False(t, session.MaxPositiveGradePercent.Known)
	assert.False(t, session.MaxNegativeGradePercent.Known)
	assert.False(t, session.MinAltitudeMetres.Known)
	assert.False(t, session.MaxAltitudeMetres.Known)
	assert.False(t, session.AverageAltitudeMetres.Known)
	assert.Empty(t, session.Sport)
	assert.Empty(t, session.SubSport)
	assert.Nil(t, session.HeartRateZoneSeconds)
	assert.Nil(t, session.HeartRateZoneHighBPM)
	assert.Nil(t, session.PowerZoneSeconds)
	assert.Nil(t, session.PowerZoneHighWatts)
	assert.False(t, session.Any())
}

// wahooDeveloperDataID is one developer_data_id message declaring Wahoo as the
// developer behind dataIndex, which is what lets a field_description under it
// resolve by manufacturer rather than by a fixed field number.
func wahooDeveloperDataID(dataIndex uint8) *mesgdef.DeveloperDataId {
	return mesgdef.NewDeveloperDataId(nil).
		SetDeveloperDataIndex(dataIndex).
		SetManufacturerId(typedef.ManufacturerWahooFitness)
}

// wahooFieldDescription names one developer field the way Wahoo's own FIT
// files do, under the given developer_data_id.
func wahooFieldDescription(dataIndex, fieldNum uint8, name string) *mesgdef.FieldDescription {
	return mesgdef.NewFieldDescription(nil).
		SetDeveloperDataIndex(dataIndex).
		SetFieldDefinitionNumber(fieldNum).
		SetFieldName([]string{name}).
		SetFitBaseTypeId(basetype.Uint16)
}

func TestDecodeFITReadsSpeedGradeCaloriesAndWahooAscentDescent(t *testing.T) {
	activity := &filedef.Activity{}
	activity.FileId.SetType(typedef.FileActivity).SetManufacturer(typedef.ManufacturerWahooFitness)
	activity.DeveloperDataIds = append(activity.DeveloperDataIds, wahooDeveloperDataID(0))
	activity.FieldDescriptions = append(activity.FieldDescriptions,
		wahooFieldDescription(0, 0, "ascent"), wahooFieldDescription(0, 1, "descent"))
	activity.Records = append(activity.Records, mesgdef.NewRecord(nil).
		SetTimestamp(time.Date(2026, time.August, 1, 6, 0, 0, 0, time.UTC)).
		SetEnhancedSpeedScaled(8.5).
		SetGradeScaled(4.2).
		SetCalories(320).
		SetDeveloperFields(
			proto.DeveloperField{Num: 0, DeveloperDataIndex: 0, Value: proto.Uint16(120)},
			proto.DeveloperField{Num: 1, DeveloperDataIndex: 0, Value: proto.Uint16(45)},
		))

	decoded, err := DecodeFIT(encodeDeveloperFields(t, activity))
	require.NoError(t, err)
	require.Len(t, decoded.Records, 1)
	record := decoded.Records[0]
	assert.True(t, record.HasSpeed)
	assert.InDelta(t, 8.5, record.SpeedMS, 0.001)
	assert.True(t, record.HasGrade)
	assert.InDelta(t, 4.2, record.GradePercent, 0.01)
	assert.True(t, record.HasCalories)
	assert.InDelta(t, 320, record.CaloriesKcal, 0)
	assert.True(t, record.HasAscent)
	assert.InDelta(t, 120, record.AscentMetres, 0)
	assert.True(t, record.HasDescent)
	assert.InDelta(t, 45, record.DescentMetres, 0)
}

// A developer field's own FIT base type decides which proto.Value accessor
// carries it; ascent and descent must read back the same regardless of which
// integer or float type a device chose to encode them as.
func TestDeveloperFieldFloat64ReadsEveryNumericBaseType(t *testing.T) {
	cases := map[string]struct {
		value proto.Value
		want  float64
	}{
		"int8":    {value: proto.Int8(-5), want: -5},
		"uint8":   {value: proto.Uint8(5), want: 5},
		"int16":   {value: proto.Int16(-500), want: -500},
		"uint16":  {value: proto.Uint16(500), want: 500},
		"int32":   {value: proto.Int32(-70000), want: -70000},
		"uint32":  {value: proto.Uint32(70000), want: 70000},
		"int64":   {value: proto.Int64(-5000000000), want: -5000000000},
		"uint64":  {value: proto.Uint64(5000000000), want: 5000000000},
		"float32": {value: proto.Float32(1.5), want: 1.5},
		"float64": {value: proto.Float64(2.5), want: 2.5},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			value, ok := developerFieldFloat64(tc.value)
			require.True(t, ok)
			assert.InDelta(t, tc.want, value, 0.001)
		})
	}

	_, ok := developerFieldFloat64(proto.String("not a number"))
	assert.False(t, ok, "a non-numeric developer field must be reported as unreadable, not zero")
}

func TestDecodeFITFallsBackToPlainSpeedWhenEnhancedIsAbsent(t *testing.T) {
	record := fromRecord(mesgdef.NewRecord(nil).SetSpeedScaled(3.3), nil)
	assert.True(t, record.HasSpeed)
	assert.InDelta(t, 3.3, record.SpeedMS, 0.001)
}

func TestDecodeFITLeavesSpeedGradeCaloriesAscentDescentAbsentWhenUnset(t *testing.T) {
	activity := &filedef.Activity{}
	activity.FileId.SetType(typedef.FileActivity)
	activity.Records = append(activity.Records, mesgdef.NewRecord(nil).
		SetTimestamp(time.Date(2026, time.August, 1, 6, 0, 0, 0, time.UTC)))

	decoded, err := DecodeFIT(encode(t, activity))
	require.NoError(t, err)
	require.Len(t, decoded.Records, 1)
	record := decoded.Records[0]
	assert.False(t, record.HasSpeed)
	assert.False(t, record.HasGrade)
	assert.False(t, record.HasCalories)
	assert.False(t, record.HasAscent)
	assert.False(t, record.HasDescent)
}

// A developer field that is not Wahoo's own — no field_description names it —
// must decode cleanly and leave ascent and descent absent, even when it
// happens to share a field number with what Wahoo would use.
func TestDecodeFITIgnoresAnUnrelatedDeveloperField(t *testing.T) {
	activity := &filedef.Activity{}
	activity.FileId.SetType(typedef.FileActivity)
	activity.DeveloperDataIds = append(activity.DeveloperDataIds,
		mesgdef.NewDeveloperDataId(nil).SetDeveloperDataIndex(0).SetManufacturerId(typedef.ManufacturerGarmin))
	activity.FieldDescriptions = append(activity.FieldDescriptions,
		mesgdef.NewFieldDescription(nil).
			SetDeveloperDataIndex(0).SetFieldDefinitionNumber(0).SetFieldName([]string{"some_other_field"}).
			SetFitBaseTypeId(basetype.Uint16))
	activity.Records = append(activity.Records, mesgdef.NewRecord(nil).
		SetTimestamp(time.Date(2026, time.August, 1, 6, 0, 0, 0, time.UTC)).
		SetDeveloperFields(proto.DeveloperField{Num: 0, DeveloperDataIndex: 0, Value: proto.Uint16(7)}))

	decoded, err := DecodeFIT(encodeDeveloperFields(t, activity))
	require.NoError(t, err)
	require.Len(t, decoded.Records, 1)
	assert.False(t, decoded.Records[0].HasAscent)
	assert.False(t, decoded.Records[0].HasDescent)
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

// encodeDeveloperFields is encode, but under protocol version 2.0: developer
// fields are a protocol violation under the 1.0 that encode defaults to.
func encodeDeveloperFields(t *testing.T, file filedef.File) []byte {
	t.Helper()
	fit := file.ToFIT(nil)
	var buffer bytes.Buffer
	require.NoError(t, fitencoder.New(&buffer, fitencoder.WithProtocolVersion(proto.V2)).Encode(&fit))

	return buffer.Bytes()
}
