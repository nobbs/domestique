// Package activity decodes recorded cycling activity FIT files.
package activity

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/muktihari/fit/profile/untyped/mesgnum"
	"github.com/muktihari/fit/proto"
)

// FIT is the activity data Domestique retains from one FIT file.
type FIT struct {
	RecordingDevice string
	Records         []Record
	Session         Session
	ChecksumFailed  bool
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
	TargetPowerWatts      float64
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
	HasTargetPower        bool
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
	namedFields := developerFieldNames(activity)
	records := make([]Record, len(activity.Records))
	for i, record := range activity.Records {
		records[i] = fromRecord(record, wahooFields, namedFields)
	}

	decoded := FIT{Records: records, RecordingDevice: activity.FileId.Manufacturer.String()}
	if len(activity.Sessions) == 0 {
		return decoded
	}

	decoded.Session = sessionFromMesg(activity.Sessions[0], activity.UnrelatedMessages)

	return decoded
}

// known wraps a value already checked against its FIT invalid sentinel into a
// Reading.
func known(value float64) Reading {
	return Reading{Value: value, Known: true}
}

func sessionFromMesg(session *mesgdef.Session, unrelated []proto.Message) Session {
	var result Session
	if session.TotalDistance != basetype.Uint32Invalid {
		result.DistanceMetres = known(session.TotalDistanceScaled())
	}
	if session.TotalTimerTime != basetype.Uint32Invalid {
		result.TimerSeconds = known(session.TotalTimerTimeScaled())
	}
	if session.TotalElapsedTime != basetype.Uint32Invalid {
		result.ElapsedSeconds = known(session.TotalElapsedTimeScaled())
	}
	sessionSpeed(session, &result)
	sessionTotals(session, &result)
	sessionHeartRateAndCadence(session, &result)
	sessionPower(session, &result)
	sessionTemperatureAndGrade(session, &result)
	sessionAltitude(session, &result)
	if session.Sport != typedef.SportInvalid {
		result.Sport = session.Sport.String()
	}
	if session.SubSport != typedef.SubSportInvalid {
		result.SubSport = session.SubSport.String()
	}
	result.HeartRateZoneSeconds = session.TimeInHrZoneScaled()
	result.PowerZoneSeconds = session.TimeInPowerZoneScaled()
	result.HeartRateZoneHighBPM = hrZoneHighs(unrelated)
	result.PowerZoneHighWatts = powerZoneHighs(unrelated)

	return result
}

func sessionSpeed(session *mesgdef.Session, result *Session) {
	if session.EnhancedMaxSpeed != basetype.Uint32Invalid {
		result.MaxSpeedKmh = known(session.EnhancedMaxSpeedScaled() * 3.6)
	} else if session.MaxSpeed != basetype.Uint16Invalid {
		result.MaxSpeedKmh = known(session.MaxSpeedScaled() * 3.6)
	}
	if session.EnhancedAvgSpeed != basetype.Uint32Invalid {
		result.AverageSpeedKmh = known(session.EnhancedAvgSpeedScaled() * 3.6)
	} else if session.AvgSpeed != basetype.Uint16Invalid {
		result.AverageSpeedKmh = known(session.AvgSpeedScaled() * 3.6)
	}
}

func sessionTotals(session *mesgdef.Session, result *Session) {
	if session.TotalAscent != basetype.Uint16Invalid {
		result.AscentMetres = known(float64(session.TotalAscent))
	}
	if session.TotalDescent != basetype.Uint16Invalid {
		result.DescentMetres = known(float64(session.TotalDescent))
	}
	if session.TotalCalories != basetype.Uint16Invalid {
		result.CaloriesKcal = known(float64(session.TotalCalories))
	}
}

func sessionHeartRateAndCadence(session *mesgdef.Session, result *Session) {
	if session.AvgHeartRate != basetype.Uint8Invalid {
		result.AverageHeartRateBPM = known(float64(session.AvgHeartRate))
	}
	if session.MaxHeartRate != basetype.Uint8Invalid {
		result.MaxHeartRateBPM = known(float64(session.MaxHeartRate))
	}
	if session.MinHeartRate != basetype.Uint8Invalid {
		result.MinHeartRateBPM = known(float64(session.MinHeartRate))
	}
	if session.AvgCadence != basetype.Uint8Invalid {
		result.AverageCadenceRPM = known(float64(session.AvgCadence))
	}
	if session.MaxCadence != basetype.Uint8Invalid {
		result.MaxCadenceRPM = known(float64(session.MaxCadence))
	}
}

func sessionPower(session *mesgdef.Session, result *Session) {
	if session.AvgPower != basetype.Uint16Invalid {
		result.AveragePowerWatts = known(float64(session.AvgPower))
	}
	if session.MaxPower != basetype.Uint16Invalid {
		result.MaxPowerWatts = known(float64(session.MaxPower))
	}
	if session.NormalizedPower != basetype.Uint16Invalid {
		result.NormalizedPowerWatts = known(float64(session.NormalizedPower))
	}
	if session.ThresholdPower != basetype.Uint16Invalid {
		result.ThresholdPowerWatts = known(float64(session.ThresholdPower))
	}
}

func sessionTemperatureAndGrade(session *mesgdef.Session, result *Session) {
	if session.AvgTemperature != basetype.Sint8Invalid {
		result.AverageTemperatureCelsius = known(float64(session.AvgTemperature))
	}
	if session.MaxTemperature != basetype.Sint8Invalid {
		result.MaxTemperatureCelsius = known(float64(session.MaxTemperature))
	}
	if session.AvgGrade != basetype.Sint16Invalid {
		result.AverageGradePercent = known(session.AvgGradeScaled())
	}
	if session.MaxPosGrade != basetype.Sint16Invalid {
		result.MaxPositiveGradePercent = known(session.MaxPosGradeScaled())
	}
	if session.MaxNegGrade != basetype.Sint16Invalid {
		result.MaxNegativeGradePercent = known(session.MaxNegGradeScaled())
	}
}

// sessionAltitude follows the same enhanced-field-wins pattern as the record
// altitude() helper.
func sessionAltitude(session *mesgdef.Session, result *Session) {
	if session.EnhancedMinAltitude != basetype.Uint32Invalid {
		result.MinAltitudeMetres = known(session.EnhancedMinAltitudeScaled())
	} else if session.MinAltitude != basetype.Uint16Invalid {
		result.MinAltitudeMetres = known(session.MinAltitudeScaled())
	}
	if session.EnhancedMaxAltitude != basetype.Uint32Invalid {
		result.MaxAltitudeMetres = known(session.EnhancedMaxAltitudeScaled())
	} else if session.MaxAltitude != basetype.Uint16Invalid {
		result.MaxAltitudeMetres = known(session.MaxAltitudeScaled())
	}
	if session.EnhancedAvgAltitude != basetype.Uint32Invalid {
		result.AverageAltitudeMetres = known(session.EnhancedAvgAltitudeScaled())
	} else if session.AvgAltitude != basetype.Uint16Invalid {
		result.AverageAltitudeMetres = known(session.AvgAltitudeScaled())
	}
}

// zone is one hr_zone or power_zone message's upper bound, kept with its
// message_index since the FIT profile groups zones by number, not by order.
type zone struct {
	index typedef.MessageIndex
	high  float64
}

// zoneHighs sorts zones by message_index and returns nil, not an empty slice,
// when the activity carried none.
func zoneHighs(zones []zone) []float64 {
	if len(zones) == 0 {
		return nil
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].index < zones[j].index })

	highs := make([]float64, len(zones))
	for i := range zones {
		highs[i] = zones[i].high
	}

	return highs
}

// hrZoneHighs collects each hr_zone message's upper bound from the messages
// an activity did not otherwise recognise, in message_index order.
func hrZoneHighs(unrelated []proto.Message) []float64 {
	var zones []zone
	for i := range unrelated {
		if unrelated[i].Num != mesgnum.HrZone {
			continue
		}
		mesg := mesgdef.NewHrZone(&unrelated[i])
		if mesg.HighBpm == basetype.Uint8Invalid {
			continue
		}
		zones = append(zones, zone{mesg.MessageIndex, float64(mesg.HighBpm)})
	}

	return zoneHighs(zones)
}

// powerZoneHighs is hrZoneHighs for power_zone messages.
func powerZoneHighs(unrelated []proto.Message) []float64 {
	var zones []zone
	for i := range unrelated {
		if unrelated[i].Num != mesgnum.PowerZone {
			continue
		}
		mesg := mesgdef.NewPowerZone(&unrelated[i])
		if mesg.HighValue == basetype.Uint16Invalid {
			continue
		}
		zones = append(zones, zone{mesg.MessageIndex, float64(mesg.HighValue)})
	}

	return zoneHighs(zones)
}

func fromRecord(record *mesgdef.Record, wahooFields, namedFields map[devFieldKey]string) Record {
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
	applyNamedDeveloperFields(record, namedFields, &decoded)

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

// developerFieldNames maps every developer field a FIT's field_descriptions
// declare to its name, regardless of which developer_data_id's manufacturer
// it belongs to. Unlike wahooDeveloperFieldNames, this is used for fields such
// as Zwift's target_power whose developer_data_id carries no manufacturer.
func developerFieldNames(activity *filedef.Activity) map[devFieldKey]string {
	names := make(map[devFieldKey]string, len(activity.FieldDescriptions))
	for _, description := range activity.FieldDescriptions {
		if len(description.FieldName) == 0 {
			continue
		}
		key := devFieldKey{description.DeveloperDataIndex, description.FieldDefinitionNumber}
		names[key] = description.FieldName[0]
	}

	return names
}

// applyNamedDeveloperFields fills the record fields matched by name alone,
// currently only the target power a structured workout prescribed.
func applyNamedDeveloperFields(record *mesgdef.Record, namedFields map[devFieldKey]string, decoded *Record) {
	for _, field := range record.DeveloperFields {
		name, known := namedFields[devFieldKey{field.DeveloperDataIndex, field.Num}]
		if !known || name != "target_power" {
			continue
		}
		// Zwift's own field is a uint16; its FIT invalid sentinel is absent,
		// never a real zero.
		if raw, ok := field.Value.Any().(uint16); ok && raw == basetype.Uint16Invalid {
			continue
		}
		value, ok := developerFieldFloat64(field.Value)
		if !ok {
			continue
		}
		decoded.HasTargetPower, decoded.TargetPowerWatts = true, value
	}
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
