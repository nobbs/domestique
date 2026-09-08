package activity

import "time"

// SeriesName is one of the recorded series a ride's samples can be read as,
// spelled as the served surface names it.
type SeriesName string

// The series a ride's samples can answer. Speed is served as derived rather
// than recorded, from distance against time, even on a device that stores its
// own speed reading: this series is one figure, not two disagreeing ones.
const (
	SeriesHeartRate   SeriesName = "heartRate"
	SeriesCadence     SeriesName = "cadence"
	SeriesPower       SeriesName = "power"
	SeriesTemperature SeriesName = "temperature"
	SeriesSpeed       SeriesName = "speed"
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
	// SpeedMS, GradePercent, CaloriesKcal, AscentMetres and DescentMetres are
	// the device's own readings, kept alongside without feeding any served
	// figure: the estimate and the derived speed series stay their own numbers.
	SpeedMS       Reading
	GradePercent  Reading
	CaloriesKcal  Reading
	AscentMetres  Reading
	DescentMetres Reading
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
		case SeriesSpeed:
		}
		present = present || readings[index].Known
	}

	return readings, present
}

// speedSeries is how fast the rider was travelling at each sample, in
// kilometres per hour, from the distance covered since the sample before it.
//
// The first sample has no interval behind it, and a pair whose clock did not
// advance — or whose odometer went backwards over a reset — has no speed to
// report rather than an infinite or negative one.
func speedSeries(rows []SampleRow) (readings []Reading, present bool) {
	readings = make([]Reading, len(rows))
	for index := 1; index < len(rows); index++ {
		previous, current := &rows[index-1], &rows[index]
		if !previous.DistanceMetres.Known || !current.DistanceMetres.Known {
			continue
		}
		seconds := current.Time.Sub(previous.Time).Seconds()
		metres := current.DistanceMetres.Value - previous.DistanceMetres.Value
		if seconds <= 0 || metres < 0 {
			continue
		}
		readings[index] = Reading{Value: metres / seconds * 3.6, Known: true}
		present = true
	}

	return readings, present
}
