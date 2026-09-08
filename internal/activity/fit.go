// Package activity decodes recorded cycling activity FIT files.
package activity

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/proto"
)

// FIT is the activity data Domestique retains from one FIT file.
type FIT struct {
	RecordingDevice     string
	Records             []Record
	TotalTimerTime      time.Duration
	TotalElapsedTime    time.Duration
	TotalAscentMetres   float64
	ChecksumFailed      bool
	HasTotalTimerTime   bool
	HasTotalElapsedTime bool
	HasTotalAscent      bool
}

// Record is one timestamped sample from an activity FIT file.
type Record struct {
	Time                  time.Time
	CadenceRPM            float64
	HeartRateBPM          float64
	Latitude              float64
	Longitude             float64
	PowerWatts            float64
	AltitudeMetres        float64
	TemperatureCelsius    float64
	DistanceMetres        float64
	SpeedMS               float64
	GradePercent          float64
	CaloriesKcal          float64
	AscentMetres          float64
	DescentMetres         float64
	HasCadence            bool
	HasTemperatureCelsius bool
	HasDistance           bool
	HasPower              bool
	HasAltitude           bool
	HasHeartRate          bool
	HasPosition           bool
	HasSpeed              bool
	HasGrade              bool
	HasCalories           bool
	HasAscent             bool
	HasDescent            bool
}

// DecodeFIT decodes an activity FIT file. A bad checksum is retained as a
// diagnostic because some device exports remain otherwise readable.
func DecodeFIT(raw []byte) (FIT, error) {
	activity, err := decode(raw)
	if err == nil {
		return activity, nil
	}
	if !errors.Is(err, decoder.ErrCRCChecksumMismatch) {
		return FIT{}, fmt.Errorf("decoding FIT: %w", err)
	}

	activity, err = decode(raw, decoder.WithIgnoreChecksum())
	if err != nil {
		return FIT{}, fmt.Errorf("decoding FIT with checksum ignored: %w", err)
	}
	activity.ChecksumFailed = true

	return activity, nil
}

func decode(raw []byte, opts ...decoder.Option) (FIT, error) {
	listener := filedef.NewListener()
	defer listener.Close()
	fitDecoder := decoder.New(bytes.NewReader(raw), append([]decoder.Option{decoder.WithMesgListener(listener)}, opts...)...)
	if _, err := fitDecoder.Decode(); err != nil {
		return FIT{}, fmt.Errorf("decoding: %w", err)
	}

	activity, ok := listener.File().(*filedef.Activity)
	if !ok || activity == nil {
		return FIT{}, errors.New("no activity file type found")
	}

	return fromActivity(activity), nil
}

func fromActivity(activity *filedef.Activity) FIT {
	wahooFields := wahooDeveloperFieldNames(activity)
	records := make([]Record, len(activity.Records))
	for i, record := range activity.Records {
		records[i] = fromRecord(record, wahooFields)
	}

	decoded := FIT{Records: records, RecordingDevice: activity.FileId.Manufacturer.String()}
	if len(activity.Sessions) == 0 {
		return decoded
	}

	session := activity.Sessions[0]
	if session.TotalTimerTime != basetype.Uint32Invalid {
		decoded.HasTotalTimerTime = true
		decoded.TotalTimerTime = time.Duration(session.TotalTimerTimeScaled() * float64(time.Second))
	}
	if session.TotalElapsedTime != basetype.Uint32Invalid {
		decoded.HasTotalElapsedTime = true
		decoded.TotalElapsedTime = time.Duration(session.TotalElapsedTimeScaled() * float64(time.Second))
	}
	if session.TotalAscent != basetype.Uint16Invalid {
		decoded.HasTotalAscent = true
		decoded.TotalAscentMetres = float64(session.TotalAscent)
	}

	return decoded
}

func fromRecord(record *mesgdef.Record, wahooFields map[devFieldKey]string) Record {
	decoded := Record{Time: record.Timestamp}
	if record.PositionLat != basetype.Sint32Invalid && record.PositionLong != basetype.Sint32Invalid {
		decoded.HasPosition = true
		decoded.Latitude = record.PositionLatDegrees()
		decoded.Longitude = record.PositionLongDegrees()
	}
	if altitude, ok := altitude(record); ok {
		decoded.HasAltitude = true
		decoded.AltitudeMetres = altitude
	}
	if record.Distance != basetype.Uint32Invalid {
		decoded.HasDistance = true
		decoded.DistanceMetres = record.DistanceScaled()
	}
	if record.Cadence != basetype.Uint8Invalid {
		decoded.HasCadence = true
		decoded.CadenceRPM = float64(record.Cadence)
	}
	if record.Temperature != basetype.Sint8Invalid {
		decoded.HasTemperatureCelsius = true
		decoded.TemperatureCelsius = float64(record.Temperature)
	}
	if record.Power != basetype.Uint16Invalid {
		decoded.HasPower = true
		decoded.PowerWatts = float64(record.Power)
	}
	if record.HeartRate != basetype.Uint8Invalid {
		decoded.HasHeartRate = true
		decoded.HeartRateBPM = float64(record.HeartRate)
	}
	if speed, ok := speedMS(record); ok {
		decoded.HasSpeed = true
		decoded.SpeedMS = speed
	}
	if record.Grade != basetype.Sint16Invalid {
		decoded.HasGrade = true
		decoded.GradePercent = record.GradeScaled()
	}
	if record.Calories != basetype.Uint16Invalid {
		decoded.HasCalories = true
		decoded.CaloriesKcal = float64(record.Calories)
	}
	applyWahooDeveloperFields(record, wahooFields, &decoded)

	return decoded
}

func altitude(record *mesgdef.Record) (float64, bool) {
	if record.EnhancedAltitude != basetype.Uint32Invalid {
		return record.EnhancedAltitudeScaled(), true
	}
	if record.Altitude != basetype.Uint16Invalid {
		return record.AltitudeScaled(), true
	}

	return 0, false
}

// speedMS falls back to the legacy Speed field when a device left the
// enhanced one, which every modern export fills, unset.
func speedMS(record *mesgdef.Record) (float64, bool) {
	if record.EnhancedSpeed != basetype.Uint32Invalid {
		return record.EnhancedSpeedScaled(), true
	}
	if record.Speed != basetype.Uint16Invalid {
		return record.SpeedScaled(), true
	}

	return 0, false
}

// devFieldKey identifies one developer field definition the way a FIT file
// itself scopes it: by which developer declared it and which number they gave
// it, neither of which is stable across files.
type devFieldKey struct {
	dataIndex byte
	fieldNum  byte
}

// wahooDeveloperFieldNames maps each developer field Wahoo's own
// developer_data_id declares to the name its field_description gives it, so
// records can be matched by name rather than by a number Wahoo could reassign.
func wahooDeveloperFieldNames(activity *filedef.Activity) map[devFieldKey]string {
	wahooIndexes := make(map[byte]bool, len(activity.DeveloperDataIds))
	for _, id := range activity.DeveloperDataIds {
		if id.ManufacturerId == typedef.ManufacturerWahooFitness {
			wahooIndexes[id.DeveloperDataIndex] = true
		}
	}

	names := make(map[devFieldKey]string, len(activity.FieldDescriptions))
	for _, description := range activity.FieldDescriptions {
		if !wahooIndexes[description.DeveloperDataIndex] || len(description.FieldName) == 0 {
			continue
		}
		key := devFieldKey{description.DeveloperDataIndex, description.FieldDefinitionNumber}
		names[key] = description.FieldName[0]
	}

	return names
}

// applyWahooDeveloperFields fills the cumulative ascent and descent Wahoo
// devices report as developer fields rather than through the FIT profile.
func applyWahooDeveloperFields(record *mesgdef.Record, wahooFields map[devFieldKey]string, decoded *Record) {
	for _, field := range record.DeveloperFields {
		name, known := wahooFields[devFieldKey{field.DeveloperDataIndex, field.Num}]
		if !known {
			continue
		}
		value, ok := developerFieldFloat64(field.Value)
		if !ok {
			continue
		}
		switch name {
		case "ascent":
			decoded.HasAscent, decoded.AscentMetres = true, value
		case "descent":
			decoded.HasDescent, decoded.DescentMetres = true, value
		}
	}
}

// developerFieldFloat64 reads a developer field's value as float64 regardless
// of the integer or float base type the file declared it with.
func developerFieldFloat64(value proto.Value) (float64, bool) {
	switch v := value.Any().(type) {
	case int8:
		return float64(v), true
	case uint8:
		return float64(v), true
	case int16:
		return float64(v), true
	case uint16:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		return float64(v), true
	case float64:
		return v, true
	default:
		return 0, false
	}
}
