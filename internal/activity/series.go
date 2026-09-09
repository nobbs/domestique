package activity

import "time"

// SeriesName is one of the recorded series a ride's samples can be read as,
// spelled as the served surface names it.
type SeriesName string

// The series a ride's samples can answer. Speed prefers what the device
// itself recorded, falling back sample by sample to distance against time
// wherever a record carries none.
const (
	SeriesHeartRate   SeriesName = "heartRate"
	SeriesCadence     SeriesName = "cadence"
	SeriesPower       SeriesName = "power"
	SeriesTemperature SeriesName = "temperature"
	SeriesSpeed       SeriesName = "speed"
	SeriesTargetPower SeriesName = "targetPower"
)

// Reading is one sample of a series. Known is false where that sample recorded
// nothing, which is not the same as recording a zero: a stopped rider's cadence
// is nought, and an unfitted sensor's is nothing at all.
type Reading struct {
	Value float64
	Known bool
}

// SampleRow is the non-positional part of one positioned sample, in the order
// it was recorded. Every field is optional because a FIT record carries only
// what the bicycle was fitted to measure.
type SampleRow struct {
	Time               time.Time
	DistanceMetres     Reading
	AltitudeMetres     Reading
	HeartRateBPM       Reading
	CadenceRPM         Reading
	PowerWatts         Reading
	TemperatureCelsius Reading
	// SpeedMS is the device's own reading, in metres per second, which the
	// speed series prefers over deriving one. GradePercent, CaloriesKcal,
	// AscentMetres and DescentMetres are kept alongside without feeding any
	// served figure: the estimate stays its own number.
	SpeedMS       Reading
	GradePercent  Reading
	CaloriesKcal  Reading
	AscentMetres  Reading
	DescentMetres Reading
	// TargetPowerWatts is the power a structured workout prescribed for this
	// record, decoded from a Zwift FIT's own developer field.
	TargetPowerWatts Reading
}

// Series reads one named series off a ride's samples, one reading per row in
// the row order. present is false when no row carried the series at all, which
// is what tells "this bicycle has no meter" from "the meter dropped out".
func Series(rows []SampleRow, name SeriesName) (readings []Reading, present bool) {
	if name == SeriesSpeed {
		return speedSeries(rows)
	}
	readings = make([]Reading, len(rows))
	for index := range rows {
		switch name {
		case SeriesHeartRate:
			readings[index] = rows[index].HeartRateBPM
		case SeriesCadence:
			readings[index] = rows[index].CadenceRPM
		case SeriesPower:
			readings[index] = rows[index].PowerWatts
		case SeriesTemperature:
			readings[index] = rows[index].TemperatureCelsius
		case SeriesTargetPower:
			readings[index] = rows[index].TargetPowerWatts
		case SeriesSpeed:
		}
		present = present || readings[index].Known
	}

	return readings, present
}

// speedSeries is how fast the rider was travelling at each sample, in
// kilometres per hour: the device's own reading where a record carries one,
// and the distance covered since the sample before it everywhere else — a
// dropout the device leaves for one sample costs that sample alone.
//
// A sample with neither has no speed to report. The first sample has no
// interval behind it to derive from, and a pair whose clock did not advance —
// or whose odometer went backwards over a reset — falls back to nothing
// rather than an infinite or negative one.
func speedSeries(rows []SampleRow) (readings []Reading, present bool) {
	readings = make([]Reading, len(rows))
	for index := range rows {
		if rows[index].SpeedMS.Known {
			readings[index] = Reading{Value: rows[index].SpeedMS.Value * 3.6, Known: true}
			present = true

			continue
		}
		if index == 0 {
			continue
		}
		previous, current := &rows[index-1], &rows[index]
		kmh, ok := DistanceSpeedKmh(
			DistanceStep{At: previous.Time, Distance: previous.DistanceMetres.Value, Known: previous.DistanceMetres.Known},
			DistanceStep{At: current.Time, Distance: current.DistanceMetres.Value, Known: current.DistanceMetres.Known},
		)
		if !ok {
			continue
		}
		readings[index] = Reading{Value: kmh, Known: true}
		present = true
	}

	return readings, present
}

// DistanceStep is one distance reading and when it was taken, the shape
// DistanceSpeedKmh derives an odometer-based speed from.
type DistanceStep struct {
	At       time.Time
	Distance float64
	Known    bool
}

// DistanceSpeedKmh is the km/h between two distance steps, the one rule every
// odometer-derived speed follows: none where either step lacks a distance,
// the clock did not advance, or the odometer went backwards over a reset.
func DistanceSpeedKmh(previous, current DistanceStep) (kmh float64, ok bool) {
	if !previous.Known || !current.Known {
		return 0, false
	}
	seconds := current.At.Sub(previous.At).Seconds()
	metres := current.Distance - previous.Distance
	if seconds <= 0 || metres < 0 {
		return 0, false
	}

	return metres / seconds * 3.6, true
}
