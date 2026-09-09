package activity

// Session is what the device itself declared for the ride: the FIT session
// message and the zone messages beside it. Every figure is a Reading so a file
// that lacks one leaves it unknown rather than zero.
type Session struct {
	Sport                     string
	SubSport                  string
	HeartRateZoneSeconds      []float64
	HeartRateZoneHighBPM      []float64
	PowerZoneSeconds          []float64
	PowerZoneHighWatts        []float64
	AverageHeartRateBPM       Reading
	AveragePowerWatts         Reading
	DistanceMetres            Reading
	TimerSeconds              Reading
	ElapsedSeconds            Reading
	AscentMetres              Reading
	DescentMetres             Reading
	CaloriesKcal              Reading
	MaxSpeedKmh               Reading
	MaxHeartRateBPM           Reading
	MinHeartRateBPM           Reading
	AverageCadenceRPM         Reading
	MaxCadenceRPM             Reading
	AverageSpeedKmh           Reading
	MaxPowerWatts             Reading
	NormalizedPowerWatts      Reading
	ThresholdPowerWatts       Reading
	AverageTemperatureCelsius Reading
	MaxTemperatureCelsius     Reading
	AverageGradePercent       Reading
	MaxPositiveGradePercent   Reading
	MaxNegativeGradePercent   Reading
	MinAltitudeMetres         Reading
	MaxAltitudeMetres         Reading
	AverageAltitudeMetres     Reading
}

// Any reports whether the file declared a single one of these figures.
func (s *Session) Any() bool {
	for _, reading := range []Reading{
		s.MaxSpeedKmh, s.AverageSpeedKmh, s.DistanceMetres, s.TimerSeconds, s.ElapsedSeconds,
		s.AscentMetres, s.DescentMetres, s.CaloriesKcal, s.AverageHeartRateBPM, s.MaxHeartRateBPM,
		s.MinHeartRateBPM, s.AverageCadenceRPM, s.MaxCadenceRPM, s.AveragePowerWatts, s.MaxPowerWatts,
		s.NormalizedPowerWatts, s.ThresholdPowerWatts, s.AverageTemperatureCelsius, s.MaxTemperatureCelsius,
		s.AverageGradePercent, s.MaxPositiveGradePercent, s.MaxNegativeGradePercent,
		s.MinAltitudeMetres, s.MaxAltitudeMetres, s.AverageAltitudeMetres,
	} {
		if reading.Known {
			return true
		}
	}

	return s.Sport != "" || len(s.HeartRateZoneSeconds) > 0 || len(s.HeartRateZoneHighBPM) > 0 ||
		len(s.PowerZoneSeconds) > 0 || len(s.PowerZoneHighWatts) > 0
}
