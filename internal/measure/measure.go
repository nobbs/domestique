// Package measure holds every measurement the service takes over ground and
// time: the spherical distance model, altitude profiles along a distance, and
// the gap-aware arithmetic over recorded sample series. It knows nothing of
// routes, activities or storage; callers choose the windows and thresholds.
package measure

import "time"

// Sample is one recorded moment along a track: when, how far along, how high,
// and what the crank and thermometer read, where the ride carried them.
type Sample struct {
	At                 time.Time
	DistanceMetres     float64
	AltitudeMetres     float64
	CadenceRPM         float64
	TemperatureCelsius float64
	HasCadence         bool
	HasTemperature     bool
}

// Reading is one timestamped value of one sensor series.
type Reading struct {
	At    time.Time
	Value float64
}
